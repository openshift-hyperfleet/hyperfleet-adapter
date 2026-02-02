package k8s_client

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// K8sClient defines the interface for Kubernetes operations.
// This interface allows for easy mocking in unit tests without requiring
// a real Kubernetes cluster or DryRun mode.
type K8sClient interface {
	// Resource CRUD operations

	// GetResource retrieves a single Kubernetes resource by GVK, namespace, and name.
	// Returns the resource or an error if not found.
	GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error)

	// CreateResource creates a new Kubernetes resource.
	// Returns the created resource with server-generated fields populated.
	CreateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// UpdateResource updates an existing Kubernetes resource.
	// The resource must have resourceVersion set for optimistic concurrency.
	UpdateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// DeleteResource deletes a Kubernetes resource by GVK, namespace, and name.
	DeleteResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error

	// ApplyResource creates or updates a resource (upsert operation).
	// If the resource doesn't exist, it creates it.
	// If it exists and generation differs, it updates the resource.
	// If it exists and generation matches, it skips (idempotent).
	// The resource must have a hyperfleet.io/generation annotation.
	ApplyResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// ApplyResources applies multiple resources in sequence (batch upsert).
	// Each resource is applied using ApplyResource logic.
	// Returns results for each resource. Stops on first error.
	// All resources must have a hyperfleet.io/generation annotation.
	ApplyResources(ctx context.Context, objs []*unstructured.Unstructured) ([]ApplyResourceResult, error)

	// Discovery operations

	// DiscoverResources discovers Kubernetes resources based on the Discovery configuration.
	// If Discovery.IsSingleResource() is true, it fetches a single resource by name.
	// Otherwise, it lists resources matching the label selector.
	DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery Discovery) (*unstructured.UnstructuredList, error)

	// Data extraction operations

	// ExtractFromSecret extracts a value from a Kubernetes Secret.
	// Format: namespace.name.key (namespace is required)
	ExtractFromSecret(ctx context.Context, path string) (string, error)

	// ExtractFromConfigMap extracts a value from a Kubernetes ConfigMap.
	// Format: namespace.name.key (namespace is required)
	ExtractFromConfigMap(ctx context.Context, path string) (string, error)
}

// Ensure Client implements K8sClient interface
var _ K8sClient = (*Client)(nil)
