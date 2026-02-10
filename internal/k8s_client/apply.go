package k8s_client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ApplyResource applies a single resource, handling generation comparison and create/update/recreate logic.
func (c *Client) ApplyResource(ctx context.Context, resource transport_client.ResourceToApply, opts transport_client.ApplyOptions) (*transport_client.ApplyResult, error) {
	m := resource.Manifest
	existing := resource.ExistingResource

	manifestGen := manifest.GetGenerationFromUnstructured(m)
	var existingGen int64
	if existing != nil {
		existingGen = manifest.GetGenerationFromUnstructured(existing)
	}

	decision := manifest.CompareGenerations(manifestGen, existingGen, existing != nil)

	result := &transport_client.ApplyResult{
		Operation: string(decision.Operation),
		Reason:    decision.Reason,
	}

	// Handle recreateOnChange override
	if decision.Operation == manifest.OperationUpdate && opts.RecreateOnChange {
		result.Operation = string(manifest.OperationRecreate)
		result.Reason = fmt.Sprintf("%s, recreateOnChange=true", decision.Reason)
	}

	c.log.Infof(ctx, "Resource applying: operation=%s reason=%s",
		strings.ToUpper(result.Operation), result.Reason)

	var err error
	switch manifest.Operation(result.Operation) {
	case manifest.OperationCreate:
		result.Resource, err = c.CreateResource(ctx, m)
	case manifest.OperationUpdate:
		m.SetResourceVersion(existing.GetResourceVersion())
		m.SetUID(existing.GetUID())
		result.Resource, err = c.UpdateResource(ctx, m)
	case manifest.OperationRecreate:
		result.Resource, err = c.recreateResource(ctx, existing, m)
	case manifest.OperationSkip:
		result.Resource = existing
	}

	if err != nil {
		result.Error = err
		return result, err
	}

	return result, nil
}

// ApplyResources applies a set of resources according to the given options.
func (c *Client) ApplyResources(ctx context.Context, resources []transport_client.ResourceToApply, opts transport_client.ApplyOptions) (*transport_client.ApplyResourcesResult, error) {
	results := &transport_client.ApplyResourcesResult{
		Results: make([]transport_client.ApplyResult, 0, len(resources)),
	}

	for _, res := range resources {
		result, err := c.ApplyResource(ctx, res, opts)
		if result != nil {
			results.Results = append(results.Results, *result)
		}
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// recreateResource deletes and recreates a Kubernetes resource.
func (c *Client) recreateResource(ctx context.Context, existing, newManifest *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	gvk := existing.GroupVersionKind()
	namespace := existing.GetNamespace()
	name := existing.GetName()

	c.log.Debugf(ctx, "Deleting resource for recreation")
	if err := c.DeleteResource(ctx, gvk, namespace, name); err != nil {
		return nil, fmt.Errorf("failed to delete resource for recreation: %w", err)
	}

	c.log.Debugf(ctx, "Waiting for resource deletion to complete")
	if err := c.waitForDeletion(ctx, gvk, namespace, name); err != nil {
		return nil, fmt.Errorf("failed waiting for resource deletion: %w", err)
	}

	c.log.Debugf(ctx, "Creating new resource after deletion confirmed")
	return c.CreateResource(ctx, newManifest)
}

// waitForDeletion polls until the resource is confirmed deleted or context times out.
func (c *Client) waitForDeletion(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	const pollInterval = 100 * time.Millisecond

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Warnf(ctx, "Context cancelled/timed out while waiting for deletion")
			return fmt.Errorf("context cancelled while waiting for resource deletion: %w", ctx.Err())
		case <-ticker.C:
			_, err := c.GetResource(ctx, gvk, namespace, name)
			if err != nil {
				if apierrors.IsNotFound(err) {
					c.log.Debugf(ctx, "Resource deletion confirmed")
					return nil
				}
				errCtx := logger.WithErrorField(ctx, err)
				c.log.Errorf(errCtx, "Error checking resource deletion status")
				return fmt.Errorf("error checking deletion status: %w", err)
			}
			c.log.Debugf(ctx, "Resource still exists, waiting for deletion...")
		}
	}
}
