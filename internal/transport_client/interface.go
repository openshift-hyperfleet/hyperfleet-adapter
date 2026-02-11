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
//
// Methods accept an optional TransportContext (any) for per-request routing:
//   - k8s_client ignores it (pass nil)
//   - maestro_client expects its own concrete context type
type TransportClient interface {
	// ApplyResources applies multiple Kubernetes resources.
	// Implementation details vary by backend:
	//   - k8s_client: applies resources sequentially, stopping on first error
	//   - maestro_client: bundles all resources into a single ManifestWork
	//
	// Each resource in the batch can have its own ApplyOptions (e.g., RecreateOnChange).
	// The Target field in ResourceToApply provides per-request routing context.
	// Results are returned for all processed resources.
	ApplyResources(ctx context.Context, resources []ResourceToApply) (*ApplyResourcesResult, error)

	// GetResource retrieves a single Kubernetes resource by GVK, namespace, and name.
	// The target parameter provides per-request routing context (nil for k8s_client).
	// Returns the resource or an error if not found.
	GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, target TransportContext) (*unstructured.Unstructured, error)

	// DiscoverResources discovers Kubernetes resources based on the Discovery configuration.
	// The target parameter provides per-request routing context (nil for k8s_client).
	// If Discovery.IsSingleResource() is true, it fetches a single resource by name.
	// Otherwise, it lists resources matching the label selector.
	DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery manifest.Discovery, target TransportContext) (*unstructured.UnstructuredList, error)
}
