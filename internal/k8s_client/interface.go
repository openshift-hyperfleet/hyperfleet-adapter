package k8s_client

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/resource_applier"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// K8sClient defines the interface for Kubernetes operations.
// This interface allows for easy mocking in unit tests without requiring
// a real Kubernetes cluster or DryRun mode.
//
// K8sClient embeds ResourceApplier, providing unified resource apply operations
// that work across different backends (direct K8s, Maestro, etc.).
type K8sClient interface {
	// Embed ResourceApplier for unified apply operations
	// This provides: ApplyResources, GetResource, DiscoverResources
	resource_applier.ResourceApplier

	// ApplyResource creates or updates a single Kubernetes resource based on generation comparison.
	// This is a K8sClient-specific convenience method for single resource operations.
	//
	// If the resource doesn't exist, it creates it.
	// If it exists and the generation differs, it updates (or recreates if RecreateOnChange=true).
	// If it exists and the generation matches, it skips the update (idempotent).
	//
	// The manifest must have the hyperfleet.io/generation annotation set.
	ApplyResource(ctx context.Context, newManifest *unstructured.Unstructured, existing *unstructured.Unstructured, opts *ApplyOptions) (*ApplyResult, error)

	// Resource CRUD operations (additional to ResourceApplier)

	// CreateResource creates a new Kubernetes resource.
	// Returns the created resource with server-generated fields populated.
	CreateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// UpdateResource updates an existing Kubernetes resource.
	// The resource must have resourceVersion set for optimistic concurrency.
	UpdateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// DeleteResource deletes a Kubernetes resource by GVK, namespace, and name.
	DeleteResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error

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

// Ensure Client implements ResourceApplier interface
var _ resource_applier.ResourceApplier = (*Client)(nil)
