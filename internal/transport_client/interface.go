package transport_client

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TransportClient defines the interface for applying Kubernetes resources.
// This interface abstracts the underlying implementation, allowing resources
// to be applied via different backends:
//   - Direct Kubernetes API (k8s_client)
//   - Maestro/OCM ManifestWork (maestro_client)
//   - Other backends (GitOps, Argo, etc.)
//
// All implementations must support generation-aware apply operations:
//   - Create if resource doesn't exist
//   - Update if generation changed
//   - Skip if generation matches (idempotent)
type TransportClient interface {
	// ApplyResources applies multiple Kubernetes resources.
	// Implementation details vary by backend:
	//   - k8s_client: applies resources sequentially, stopping on first error
	//   - maestro_client: bundles all resources into a single ManifestWork
	//
	// Each resource in the batch can have its own ApplyOptions (e.g., RecreateOnChange).
	// Results are returned for all processed resources.
	//
	// Parameters:
	//   - ctx: Context for the operation
	//   - resources: List of resources to apply (must have generation annotations)
	//
	// Returns:
	//   - ApplyResourcesResult containing results for all processed resources
	//   - Error if any resource fails (results will contain partial results up to failure)
	ApplyResources(ctx context.Context, resources []ResourceToApply) (*ApplyResourcesResult, error)

	// GetResource retrieves a single Kubernetes resource by GVK, namespace, and name.
	// Returns the resource or an error if not found.
	GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error)

	// DiscoverResources discovers Kubernetes resources based on the Discovery configuration.
	// If Discovery.IsSingleResource() is true, it fetches a single resource by name.
	// Otherwise, it lists resources matching the label selector.
	DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery manifest.Discovery) (*unstructured.UnstructuredList, error)
}
