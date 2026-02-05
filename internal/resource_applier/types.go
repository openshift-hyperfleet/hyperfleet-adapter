// Package resource_applier provides a unified interface for applying Kubernetes resources
// across different backends (direct K8s API, Maestro/OCM ManifestWork, etc.).
package resource_applier

import (
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ApplyOptions configures the behavior of resource apply operations.
type ApplyOptions struct {
	// RecreateOnChange forces delete+create instead of update when resource exists
	// and generation has changed. Useful for resources that don't support in-place updates.
	RecreateOnChange bool
}

// ApplyResult contains the result of applying a single resource.
type ApplyResult struct {
	// Resource is the resulting Kubernetes resource after the operation
	Resource *unstructured.Unstructured

	// Operation is the operation that was performed (create, update, recreate, skip)
	Operation manifest.Operation

	// Reason explains why the operation was chosen
	Reason string
}

// ResourceToApply represents a single resource to be applied in a batch operation.
type ResourceToApply struct {
	// Name is a logical name for the resource (used in results for identification)
	Name string

	// Manifest is the desired resource state (must have generation annotation)
	Manifest *unstructured.Unstructured

	// Existing is the current resource state (nil if not found/discovered)
	Existing *unstructured.Unstructured

	// Options for this apply operation (optional, defaults to no special options)
	Options *ApplyOptions
}

// ResourceApplyResult contains the result for a single resource in a batch operation.
type ResourceApplyResult struct {
	// Name is the logical name of the resource (from ResourceToApply.Name)
	Name string

	// Kind is the Kubernetes resource kind
	Kind string

	// Namespace is the resource namespace
	Namespace string

	// ResourceName is the Kubernetes resource name (metadata.name)
	ResourceName string

	// ApplyResult contains the operation result (nil if failed before apply)
	*ApplyResult

	// Error is set if this resource failed to apply
	Error error
}

// ApplyResourcesResult contains the results of a batch apply operation.
type ApplyResourcesResult struct {
	// Results contains individual results for each resource in order
	Results []*ResourceApplyResult

	// FailedCount is the number of resources that failed
	FailedCount int

	// SuccessCount is the number of resources that succeeded
	SuccessCount int
}

// Failed returns true if any resources failed to apply.
func (r *ApplyResourcesResult) Failed() bool {
	return r.FailedCount > 0
}

// GetResult returns the result for a resource by name, or nil if not found.
func (r *ApplyResourcesResult) GetResult(name string) *ResourceApplyResult {
	for _, result := range r.Results {
		if result.Name == name {
			return result
		}
	}
	return nil
}

// GetResources returns a map of resource name to the resulting unstructured resource.
func (r *ApplyResourcesResult) GetResources() map[string]*unstructured.Unstructured {
	resources := make(map[string]*unstructured.Unstructured)
	for _, result := range r.Results {
		if result.ApplyResult != nil && result.Resource != nil {
			resources[result.Name] = result.Resource
		}
	}
	return resources
}
