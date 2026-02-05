package maestro_client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/resource_applier"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	workv1 "open-cluster-management.io/api/work/v1"
)

// ResourceApplier adapts maestro_client.Client to implement resource_applier.ResourceApplier.
// This allows the executor to use either direct K8s or Maestro/ManifestWork as the backend.
//
// The adapter:
//   - Stores all resources in a single ManifestWork
//   - Translates individual resource operations to ManifestWork operations
//   - Uses the ManifestWork's generation tracking for idempotent updates
type ResourceApplier struct {
	client       *Client
	consumerName string            // Target cluster name (Maestro consumer)
	workName     string            // Name of the ManifestWork to manage
	workLabels   map[string]string // Labels to apply to the ManifestWork
	log          logger.Logger
}

// ResourceApplierConfig configures the MaestroResourceApplier.
type ResourceApplierConfig struct {
	// ConsumerName is the target cluster name (required)
	ConsumerName string

	// WorkName is the name of the ManifestWork to create/update (required)
	WorkName string

	// WorkLabels are optional labels to add to the ManifestWork
	WorkLabels map[string]string
}

// NewResourceApplier creates a new MaestroResourceApplier.
//
// Parameters:
//   - client: The underlying Maestro client
//   - config: Configuration including consumer name and work name
//   - log: Logger instance
//
// Example:
//
//	applier := NewResourceApplier(maestroClient, &ResourceApplierConfig{
//	    ConsumerName: "cluster-1",
//	    WorkName:     "my-adapter-resources",
//	    WorkLabels:   map[string]string{"managed-by": "hyperfleet"},
//	}, log)
func NewResourceApplier(client *Client, config *ResourceApplierConfig, log logger.Logger) (*ResourceApplier, error) {
	if client == nil {
		return nil, fmt.Errorf("maestro client is required")
	}
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.ConsumerName == "" {
		return nil, fmt.Errorf("consumer name is required")
	}
	if config.WorkName == "" {
		return nil, fmt.Errorf("work name is required")
	}

	return &ResourceApplier{
		client:       client,
		consumerName: config.ConsumerName,
		workName:     config.WorkName,
		workLabels:   config.WorkLabels,
		log:          log,
	}, nil
}

// ApplyResources applies multiple resources by bundling them into a ManifestWork.
// All resources are stored in a single ManifestWork for the target cluster.
//
// The operation:
//  1. Builds a ManifestWork with all resources as manifests
//  2. Uses ApplyManifestWork for generation-aware create/update
//  3. Returns results for each resource
func (a *ResourceApplier) ApplyResources(
	ctx context.Context,
	resources []resource_applier.ResourceToApply,
) (*resource_applier.ApplyResourcesResult, error) {
	result := &resource_applier.ApplyResourcesResult{
		Results: make([]*resource_applier.ResourceApplyResult, 0, len(resources)),
	}

	if len(resources) == 0 {
		return result, nil
	}

	// Build ManifestWork from resources
	work, err := a.buildManifestWork(resources)
	if err != nil {
		return nil, fmt.Errorf("failed to build ManifestWork: %w", err)
	}

	a.log.Infof(ctx, "Applying %d resources via ManifestWork %s/%s",
		len(resources), a.consumerName, a.workName)

	// Apply the ManifestWork (create or update)
	appliedWork, err := a.client.ApplyManifestWork(ctx, a.consumerName, work)
	if err != nil {
		// Convert to result with error
		for _, r := range resources {
			resourceResult := &resource_applier.ResourceApplyResult{
				Name:         r.Name,
				Kind:         r.Manifest.GetKind(),
				Namespace:    r.Manifest.GetNamespace(),
				ResourceName: r.Manifest.GetName(),
				Error:        err,
			}
			result.Results = append(result.Results, resourceResult)
			result.FailedCount++
		}
		return result, fmt.Errorf("failed to apply ManifestWork: %w", err)
	}

	// Determine operation based on ManifestWork result
	// Since all resources are in one ManifestWork, they share the same operation
	op := a.determineOperation(appliedWork)

	// Build success results for all resources
	for _, r := range resources {
		resourceResult := &resource_applier.ResourceApplyResult{
			Name:         r.Name,
			Kind:         r.Manifest.GetKind(),
			Namespace:    r.Manifest.GetNamespace(),
			ResourceName: r.Manifest.GetName(),
			ApplyResult: &resource_applier.ApplyResult{
				Resource:  r.Manifest, // Return the manifest as the "result"
				Operation: op,
				Reason:    fmt.Sprintf("applied via ManifestWork %s", a.workName),
			},
		}
		result.Results = append(result.Results, resourceResult)
		result.SuccessCount++
	}

	a.log.Infof(ctx, "Successfully applied %d resources via ManifestWork", result.SuccessCount)
	return result, nil
}

// GetResource retrieves a resource from the ManifestWork's manifest list.
// It searches the manifests stored in the ManifestWork for a matching resource.
func (a *ResourceApplier) GetResource(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
) (*unstructured.Unstructured, error) {
	// Get the ManifestWork
	work, err := a.client.GetManifestWork(ctx, a.consumerName, a.workName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// ManifestWork doesn't exist, so resource doesn't exist
			gr := schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}
			return nil, apierrors.NewNotFound(gr, name)
		}
		return nil, err
	}

	// Search for the resource in the manifests
	for _, m := range work.Spec.Workload.Manifests {
		obj, err := manifestToUnstructured(m)
		if err != nil {
			continue
		}

		// Check if this manifest matches
		if obj.GetKind() == gvk.Kind &&
			obj.GetAPIVersion() == gvk.GroupVersion().String() &&
			obj.GetNamespace() == namespace &&
			obj.GetName() == name {
			return obj, nil
		}
	}

	// Resource not found in ManifestWork
	gr := schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}
	return nil, apierrors.NewNotFound(gr, name)
}

// DiscoverResources discovers resources within the ManifestWork that match the discovery criteria.
func (a *ResourceApplier) DiscoverResources(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	discovery manifest.Discovery,
) (*unstructured.UnstructuredList, error) {
	return a.client.DiscoverManifest(ctx, a.consumerName, a.workName, discovery)
}

// buildManifestWork creates a ManifestWork containing all resources as manifests.
func (a *ResourceApplier) buildManifestWork(resources []resource_applier.ResourceToApply) (*workv1.ManifestWork, error) {
	manifests := make([]workv1.Manifest, 0, len(resources))

	// Find the highest generation among all resources for the ManifestWork
	var maxGeneration int64
	for _, r := range resources {
		gen := manifest.GetGenerationFromUnstructured(r.Manifest)
		if gen > maxGeneration {
			maxGeneration = gen
		}
	}

	// Convert each resource to a Manifest
	for _, r := range resources {
		raw, err := json.Marshal(r.Manifest.Object)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal manifest %s: %w", r.Name, err)
		}
		manifests = append(manifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: raw},
		})
	}

	// Build the ManifestWork
	work := &workv1.ManifestWork{}
	work.Name = a.workName
	work.Namespace = a.consumerName

	// Set labels
	if a.workLabels != nil {
		work.Labels = a.workLabels
	}

	// Set generation annotation on ManifestWork
	if work.Annotations == nil {
		work.Annotations = make(map[string]string)
	}
	work.Annotations[constants.AnnotationGeneration] = fmt.Sprintf("%d", maxGeneration)

	// Set manifests
	work.Spec.Workload.Manifests = manifests

	return work, nil
}

// determineOperation determines the operation that was performed based on the ManifestWork.
func (a *ResourceApplier) determineOperation(work *workv1.ManifestWork) manifest.Operation {
	if work == nil {
		return manifest.OperationCreate
	}

	// Check if this is a new ManifestWork (no resourceVersion set by server)
	// or an updated one
	if work.ResourceVersion == "" {
		return manifest.OperationCreate
	}

	// If the generation matches what we set, it could be create or update
	// The actual determination was done in ApplyManifestWork
	return manifest.OperationUpdate
}

// manifestToUnstructured converts a workv1.Manifest to an unstructured object.
func manifestToUnstructured(m workv1.Manifest) (*unstructured.Unstructured, error) {
	if m.Raw == nil {
		return nil, fmt.Errorf("manifest has no raw data")
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(m.Raw, &obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &unstructured.Unstructured{Object: obj}, nil
}

// Ensure ResourceApplier implements resource_applier.ResourceApplier
var _ resource_applier.ResourceApplier = (*ResourceApplier)(nil)

// ConsumerName returns the target cluster name.
func (a *ResourceApplier) ConsumerName() string {
	return a.consumerName
}

// WorkName returns the ManifestWork name.
func (a *ResourceApplier) WorkName() string {
	return a.workName
}

// Client returns the underlying Maestro client.
func (a *ResourceApplier) Client() *Client {
	return a.client
}
