// Package transport_client defines the transport abstraction layer for resource operations.
//
// TransportClient decouples the executor from specific resource-application backends
// (e.g., Kubernetes direct, Maestro/OCM). Both k8s_client.Client and maestro_client.Client
// implement this interface, allowing the executor to operate uniformly.
package transport_client

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TransportClient defines the interface for resource transport operations.
// Implementations handle the details of delivering resources to the target
// infrastructure (e.g., direct K8s API, Maestro ManifestWork).
type TransportClient interface {
	// ApplyResources applies a set of resources according to the given options.
	// It handles generation comparison, create/update/recreate logic, and returns
	// the result for each resource.
	ApplyResources(ctx context.Context, resources []ResourceToApply, opts ApplyOptions) (*ApplyResourcesResult, error)

	// GetResource retrieves a single resource by GVK, namespace, and name.
	GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error)

	// DiscoverResources discovers resources based on the Discovery configuration.
	DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery Discovery) (*unstructured.UnstructuredList, error)
}

// Discovery defines the interface for resource discovery configuration.
type Discovery interface {
	// GetNamespace returns the namespace to search in.
	GetNamespace() string

	// GetName returns the resource name for single-resource discovery.
	GetName() string

	// GetLabelSelector returns the label selector string.
	GetLabelSelector() string

	// IsSingleResource returns true if discovering by name.
	IsSingleResource() bool
}
