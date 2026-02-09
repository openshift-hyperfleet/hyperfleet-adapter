package k8s_client

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/resource_applier"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Type aliases for backward compatibility.
// These types are now defined in the resource_applier package.
type (
	ApplyOptions         = resource_applier.ApplyOptions
	ApplyResult          = resource_applier.ApplyResult
	ResourceToApply      = resource_applier.ResourceToApply
	ResourceApplyResult  = resource_applier.ResourceApplyResult
	ApplyResourcesResult = resource_applier.ApplyResourcesResult
)

// ApplyResource creates or updates a Kubernetes resource based on generation comparison.
// This is the generation-aware upsert operation that mirrors maestro_client.ApplyManifestWork.
//
// If the resource doesn't exist, it creates it.
// If it exists and the generation differs, it updates (or recreates if RecreateOnChange=true).
// If it exists and the generation matches, it skips the update (idempotent).
//
// The manifest must have the hyperfleet.io/generation annotation set.
// Use manifest.ValidateManifest() to validate before calling.
//
// Parameters:
//   - ctx: Context for the operation
//   - newManifest: The desired resource state (must have generation annotation)
//   - existing: The current resource state (nil if not found/discovered)
//   - opts: Apply options (can be nil for defaults)
//
// Returns:
//   - ApplyResult containing the resource and operation details
//   - Error if the operation fails
//
// Example:
//
//	// Discover existing resource first
//	existing, _ := client.DiscoverResources(ctx, gvk, discovery)
//	var existingResource *unstructured.Unstructured
//	if len(existing.Items) > 0 {
//	    existingResource = manifest.GetLatestGenerationFromList(existing)
//	}
//
//	// Apply with generation tracking
//	result, err := client.ApplyResource(ctx, newManifest, existingResource, &ApplyOptions{
//	    RecreateOnChange: true,
//	})
func (c *Client) ApplyResource(
	ctx context.Context,
	newManifest *unstructured.Unstructured,
	existing *unstructured.Unstructured,
	opts *ApplyOptions,
) (*ApplyResult, error) {
	if newManifest == nil {
		return nil, fmt.Errorf("new manifest cannot be nil")
	}

	if opts == nil {
		opts = &ApplyOptions{}
	}

	// Get generation from new manifest
	newGen := manifest.GetGenerationFromUnstructured(newManifest)

	// Get existing generation (0 if not found)
	var existingGen int64
	if existing != nil {
		existingGen = manifest.GetGenerationFromUnstructured(existing)
	}

	// Compare generations to determine operation
	decision := manifest.CompareGenerations(newGen, existingGen, existing != nil)

	result := &ApplyResult{
		Operation: decision.Operation,
		Reason:    decision.Reason,
	}

	// Handle recreateOnChange override
	if decision.Operation == manifest.OperationUpdate && opts.RecreateOnChange {
		result.Operation = manifest.OperationRecreate
		result.Reason = fmt.Sprintf("%s, recreateOnChange=true", decision.Reason)
	}

	gvk := newManifest.GroupVersionKind()
	name := newManifest.GetName()

	c.log.Infof(ctx, "ApplyResource %s/%s: operation=%s reason=%s",
		gvk.Kind, name, result.Operation, result.Reason)

	// Execute the operation
	var err error
	switch result.Operation {
	case manifest.OperationCreate:
		result.Resource, err = c.CreateResource(ctx, newManifest)

	case manifest.OperationUpdate:
		// Preserve resourceVersion and UID from existing for update
		newManifest.SetResourceVersion(existing.GetResourceVersion())
		newManifest.SetUID(existing.GetUID())
		result.Resource, err = c.UpdateResource(ctx, newManifest)

	case manifest.OperationRecreate:
		result.Resource, err = c.recreateResource(ctx, existing, newManifest)

	case manifest.OperationSkip:
		result.Resource = existing
	}

	if err != nil {
		return nil, fmt.Errorf("failed to %s resource %s/%s: %w",
			result.Operation, gvk.Kind, name, err)
	}

	return result, nil
}

// ApplyResources applies multiple Kubernetes resources in sequence.
// This is a batch version of ApplyResource that processes resources one by one,
// stopping on first error (fail-fast behavior).
//
// Each resource in the batch can have its own ApplyOptions (e.g., RecreateOnChange).
// Resources are processed in order, and results are returned for all processed resources.
//
// Parameters:
//   - ctx: Context for the operation
//   - resources: List of resources to apply (must have generation annotations)
//
// Returns:
//   - ApplyResourcesResult containing results for all processed resources
//   - Error if any resource fails (results will contain partial results up to failure)
//
// Example:
//
//	resources := []k8s_client.ResourceToApply{
//	    {Name: "configmap", Manifest: cm, Existing: existingCM, Options: nil},
//	    {Name: "deployment", Manifest: deploy, Existing: existingDeploy, Options: &k8s_client.ApplyOptions{RecreateOnChange: true}},
//	}
//	result, err := client.ApplyResources(ctx, resources)
//	if err != nil {
//	    // Check result.Results for partial results
//	}
func (c *Client) ApplyResources(
	ctx context.Context,
	resources []ResourceToApply,
) (*ApplyResourcesResult, error) {
	result := &ApplyResourcesResult{
		Results: make([]*ResourceApplyResult, 0, len(resources)),
	}

	for _, r := range resources {
		resourceResult := &ResourceApplyResult{
			Name: r.Name,
		}

		// Validate manifest
		if r.Manifest == nil {
			resourceResult.Error = fmt.Errorf("manifest cannot be nil for resource %s", r.Name)
			result.Results = append(result.Results, resourceResult)
			result.FailedCount++
			return result, resourceResult.Error
		}

		// Extract resource info for logging and result
		resourceResult.Kind = r.Manifest.GetKind()
		resourceResult.Namespace = r.Manifest.GetNamespace()
		resourceResult.ResourceName = r.Manifest.GetName()

		// Apply the resource
		applyResult, err := c.ApplyResource(ctx, r.Manifest, r.Existing, r.Options)
		if err != nil {
			resourceResult.Error = err
			result.Results = append(result.Results, resourceResult)
			result.FailedCount++
			return result, fmt.Errorf("failed to apply resource %s: %w", r.Name, err)
		}

		resourceResult.ApplyResult = applyResult
		result.Results = append(result.Results, resourceResult)
		result.SuccessCount++

		c.log.Infof(ctx, "Applied resource %s: operation=%s", r.Name, applyResult.Operation)
	}

	c.log.Infof(ctx, "Applied %d resources successfully", result.SuccessCount)
	return result, nil
}

// recreateResource deletes and recreates a Kubernetes resource.
// It waits for the resource to be fully deleted before creating the new one
// to avoid race conditions with Kubernetes asynchronous deletion.
func (c *Client) recreateResource(
	ctx context.Context,
	existing *unstructured.Unstructured,
	newManifest *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	gvk := existing.GroupVersionKind()
	namespace := existing.GetNamespace()
	name := existing.GetName()

	// Delete the existing resource
	c.log.Debugf(ctx, "Deleting resource for recreation: %s/%s", gvk.Kind, name)
	if err := c.DeleteResource(ctx, gvk, namespace, name); err != nil {
		return nil, fmt.Errorf("failed to delete resource for recreation: %w", err)
	}

	// Wait for the resource to be fully deleted
	c.log.Debugf(ctx, "Waiting for resource deletion to complete: %s/%s", gvk.Kind, name)
	if err := c.waitForDeletion(ctx, gvk, namespace, name); err != nil {
		return nil, fmt.Errorf("failed waiting for resource deletion: %w", err)
	}

	// Create the new resource
	c.log.Debugf(ctx, "Creating new resource after deletion confirmed: %s/%s", gvk.Kind, name)
	return c.CreateResource(ctx, newManifest)
}

// waitForDeletion polls until the resource is confirmed deleted or context times out.
// Returns nil when the resource is confirmed gone (NotFound), or an error otherwise.
func (c *Client) waitForDeletion(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
) error {
	const pollInterval = 100 * time.Millisecond

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Warnf(ctx, "Context cancelled/timed out while waiting for deletion of %s/%s", gvk.Kind, name)
			return fmt.Errorf("context cancelled while waiting for resource deletion: %w", ctx.Err())
		case <-ticker.C:
			_, err := c.GetResource(ctx, gvk, namespace, name)
			if err != nil {
				// NotFound means the resource is deleted - this is success
				if apierrors.IsNotFound(err) {
					c.log.Debugf(ctx, "Resource deletion confirmed: %s/%s", gvk.Kind, name)
					return nil
				}
				// Any other error is unexpected
				c.log.Errorf(ctx, "Error checking deletion status for %s/%s: %v", gvk.Kind, name, err)
				return fmt.Errorf("error checking deletion status: %w", err)
			}
			// Resource still exists, continue polling
			c.log.Debugf(ctx, "Resource %s/%s still exists, waiting for deletion...", gvk.Kind, name)
		}
	}
}
