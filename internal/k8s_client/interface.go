package k8s_client

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// K8sClient defines the interface for Kubernetes operations.
// It embeds TransportClient for the standard transport abstraction layer,
// and adds Kubernetes-specific CRUD operations.
type K8sClient interface {
	// TransportClient provides ApplyResources, GetResource, and DiscoverResources
	transport_client.TransportClient

	// Kubernetes-specific resource CRUD operations

	// ApplyResource applies a single resource with generation-based comparison.
	ApplyResource(ctx context.Context, resource transport_client.ResourceToApply, opts transport_client.ApplyOptions) (*transport_client.ApplyResult, error)

	// CreateResource creates a new Kubernetes resource.
	// Returns the created resource with server-generated fields populated.
	CreateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// UpdateResource updates an existing Kubernetes resource.
	// The resource must have resourceVersion set for optimistic concurrency.
	UpdateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// DeleteResource deletes a Kubernetes resource by GVK, namespace, and name.
	DeleteResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error
}

// Ensure Client implements K8sClient interface
var _ K8sClient = (*Client)(nil)

// Ensure Client implements TransportClient interface
var _ transport_client.TransportClient = (*Client)(nil)
