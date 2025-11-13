package k8sclient

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceTracker tracks Kubernetes resources for status evaluation
type ResourceTracker struct {
	client    *Client
	resources map[string]*TrackedResource
	log       logger.Logger
}

// TrackedResource represents a tracked Kubernetes resource
type TrackedResource struct {
	// Alias is the name used to reference this resource in expressions
	Alias string
	// Resource is the actual Kubernetes resource
	Resource *unstructured.Unstructured
	// GVK is the GroupVersionKind of the resource
	GVK schema.GroupVersionKind
	// Namespace is the namespace of the resource (empty for cluster-scoped)
	Namespace string
	// Name is the name of the resource
	Name string
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker(client *Client, log logger.Logger) *ResourceTracker {
	return &ResourceTracker{
		client:    client,
		resources: make(map[string]*TrackedResource),
		log:       log,
	}
}

// TrackResource adds a resource to tracking with the given alias
func (t *ResourceTracker) TrackResource(alias string, resource *unstructured.Unstructured) {
	gvk := resource.GroupVersionKind()
	namespace := resource.GetNamespace()
	name := resource.GetName()

	t.log.V(2).Infof("Tracking resource: %s as '%s' (%s/%s in %s)", gvk.Kind, alias, namespace, name, gvk.String())

	t.resources[alias] = &TrackedResource{
		Alias:     alias,
		Resource:  resource,
		GVK:       gvk,
		Namespace: namespace,
		Name:      name,
	}
}

// GetTrackedResource retrieves a tracked resource by alias
func (t *ResourceTracker) GetTrackedResource(alias string) (*TrackedResource, bool) {
	resource, exists := t.resources[alias]
	return resource, exists
}

// GetAllTrackedResources returns all tracked resources
func (t *ResourceTracker) GetAllTrackedResources() map[string]*TrackedResource {
	return t.resources
}

// RefreshResource refreshes a tracked resource from the Kubernetes API
func (t *ResourceTracker) RefreshResource(ctx context.Context, alias string) error {
	tracked, exists := t.resources[alias]
	if !exists {
		return errors.KubernetesError("resource with alias '%s' not found in tracker", alias)
	}

	t.log.V(2).Infof("Refreshing tracked resource: %s (%s/%s)", alias, tracked.GVK.Kind, tracked.Name)

	// Fetch the latest version from Kubernetes
	refreshed, err := t.client.GetResource(ctx, tracked.GVK, tracked.Namespace, tracked.Name)
	if err != nil {
		return errors.KubernetesError("failed to refresh resource %s: %v", alias, err)
	}

	// Update the tracked resource
	tracked.Resource = refreshed
	t.resources[alias] = tracked

	t.log.V(2).Infof("Successfully refreshed resource: %s", alias)
	return nil
}

// RefreshAllResources refreshes all tracked resources from the Kubernetes API
func (t *ResourceTracker) RefreshAllResources(ctx context.Context) error {
	for alias := range t.resources {
		if err := t.RefreshResource(ctx, alias); err != nil {
			t.log.Warning(fmt.Sprintf("Failed to refresh resource %s: %v", alias, err))
			// Continue refreshing other resources even if one fails
		}
	}
	return nil
}

// DiscoverAndTrackByName discovers a resource by name and tracks it.
// Namespace and name should already be rendered by the caller (manager).
func (t *ResourceTracker) DiscoverAndTrackByName(ctx context.Context, alias string, gvk schema.GroupVersionKind, namespace, name string) error {
	// Validate inputs
	if alias == "" {
		return errors.Validation("alias is required for tracking")
	}
	if name == "" {
		return errors.Validation("resource name is required for discovery by name")
	}

	t.log.V(2).Infof("Discovering resource by name: %s/%s (namespace: %s)", gvk.Kind, name, namespace)

	resource, err := t.client.GetResource(ctx, gvk, namespace, name)
	if err != nil {
		return errors.KubernetesError("failed to get resource by name for %s: %v", alias, err)
	}

	// Track the discovered resource
	t.TrackResource(alias, resource)
	t.log.Infof("Successfully discovered and tracked resource: %s (%s/%s)", alias, gvk.Kind, resource.GetName())
	return nil
}

// DiscoverAndTrackBySelectors discovers a resource by label selectors and tracks it.
// Namespace and labelSelector should already be rendered by the caller (manager).
func (t *ResourceTracker) DiscoverAndTrackBySelectors(ctx context.Context, alias string, gvk schema.GroupVersionKind, namespace, labelSelector string) error {
	// Validate inputs
	if alias == "" {
		return errors.Validation("alias is required for tracking")
	}
	if labelSelector == "" {
		return errors.Validation("label selector is required for discovery by selectors")
	}

	t.log.V(2).Infof("Discovering resource by selectors: %s (namespace: %s, selector: %s)", gvk.Kind, namespace, labelSelector)

	list, err := t.client.ListResources(ctx, gvk, namespace, labelSelector)
	if err != nil {
		return errors.KubernetesError("failed to list resources by selectors for %s: %v", alias, err)
	}

	if len(list.Items) == 0 {
		return errors.KubernetesError("no resources found matching selectors for %s", alias)
	}

	// Use the first matching resource
	resource := &list.Items[0]
	if len(list.Items) > 1 {
		t.log.Warning(fmt.Sprintf("Multiple resources found for %s, using first one: %s", alias, resource.GetName()))
	}

	// Track the discovered resource
	t.TrackResource(alias, resource)
	t.log.Infof("Successfully discovered and tracked resource: %s (%s/%s)", alias, gvk.Kind, resource.GetName())
	return nil
}

// ExtractStatus extracts the status field from a tracked resource
func (t *ResourceTracker) ExtractStatus(alias string) (map[string]interface{}, error) {
	tracked, exists := t.resources[alias]
	if !exists {
		return nil, errors.KubernetesError("resource with alias '%s' not found in tracker", alias)
	}

	// Get the status field from the unstructured object
	status, found, err := unstructured.NestedMap(tracked.Resource.Object, "status")
	if err != nil {
		return nil, errors.KubernetesError("failed to extract status from resource %s: %v", alias, err)
	}

	if !found {
		// Resource doesn't have a status field
		return map[string]interface{}{}, nil
	}

	return status, nil
}

// ExtractField extracts a nested field from a tracked resource
func (t *ResourceTracker) ExtractField(alias string, fieldPath ...string) (interface{}, error) {
	tracked, exists := t.resources[alias]
	if !exists {
		return nil, errors.KubernetesError("resource with alias '%s' not found in tracker", alias)
	}

	value, found, err := unstructured.NestedFieldNoCopy(tracked.Resource.Object, fieldPath...)
	if err != nil {
		return nil, errors.KubernetesError("failed to extract field %v from resource %s: %v", fieldPath, alias, err)
	}

	if !found {
		return nil, errors.KubernetesError("field %v not found in resource %s", fieldPath, alias)
	}

	return value, nil
}

// BuildVariablesMap builds a map of variables for expression evaluation
// Returns a map with "resources" key containing all tracked resources
func (t *ResourceTracker) BuildVariablesMap() map[string]interface{} {
	resourcesMap := make(map[string]interface{})

	for alias, tracked := range t.resources {
		// Convert the unstructured object to a map for expression evaluation
		resourcesMap[alias] = tracked.Resource.Object
	}

	return map[string]interface{}{
		"resources": resourcesMap,
	}
}

// Clear removes all tracked resources
func (t *ResourceTracker) Clear() {
	t.resources = make(map[string]*TrackedResource)
	t.log.V(2).Info("Cleared all tracked resources")
}

// Count returns the number of tracked resources
func (t *ResourceTracker) Count() int {
	return len(t.resources)
}

