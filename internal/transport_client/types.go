package transport_client

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ApplyOptions configures the behavior of ApplyResources.
type ApplyOptions struct {
	// RecreateOnChange indicates whether to delete and recreate resources
	// when generation changes, instead of updating in place.
	RecreateOnChange bool

	// TransportConfig carries transport-specific settings (e.g., Maestro targetCluster,
	// manifestWork name, refContent) from the executor to the transport client.
	// The executor populates this without knowing about transport internals.
	TransportConfig map[string]interface{}
}

// ResourceToApply represents a single resource to be applied.
type ResourceToApply struct {
	// Manifest is the desired state of the resource.
	Manifest *unstructured.Unstructured

	// ExistingResource is the current state of the resource (if it exists).
	// nil means the resource does not exist yet.
	ExistingResource *unstructured.Unstructured
}

// ApplyResult represents the result of applying a single resource.
type ApplyResult struct {
	// Operation is the operation that was performed (create, update, recreate, skip).
	Operation string

	// Reason explains why this operation was chosen.
	Reason string

	// Resource is the resulting resource after the operation.
	Resource *unstructured.Unstructured

	// Error is the error if the operation failed.
	Error error
}

// ApplyResourcesResult represents the result of applying multiple resources.
type ApplyResourcesResult struct {
	// Results contains the result for each resource, in order.
	Results []ApplyResult
}
