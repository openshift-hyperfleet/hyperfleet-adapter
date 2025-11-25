package k8s_client

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Discovery defines the interface for resource discovery configuration.
// Any type implementing this interface can be used with Client.DiscoverResources().
type Discovery interface {
	// GetNamespace returns the namespace to search in.
	// Empty string means cluster-scoped or all namespaces.
	GetNamespace() string

	// GetName returns the resource name for single-resource discovery.
	// Empty string means use selector-based discovery.
	GetName() string

	// GetLabelSelector returns the label selector string (e.g., "app=myapp,env=prod").
	// Empty string means no label filtering.
	GetLabelSelector() string

	// IsSingleResource returns true if discovering by name (single resource).
	IsSingleResource() bool
}

// DiscoveryConfig is the default implementation of the Discovery interface.
type DiscoveryConfig struct {
	// Namespace to search in (empty for cluster-scoped or all namespaces)
	Namespace string

	// ByName specifies the resource name for single-resource discovery.
	// If set, GetResource is used instead of ListResources.
	ByName string

	// LabelSelector is the label selector string (e.g., "app=myapp,env=prod")
	LabelSelector string
}

// GetNamespace implements Discovery.GetNamespace
func (d *DiscoveryConfig) GetNamespace() string {
	return d.Namespace
}

// GetName implements Discovery.GetName
func (d *DiscoveryConfig) GetName() string {
	return d.ByName
}

// GetLabelSelector implements Discovery.GetLabelSelector
func (d *DiscoveryConfig) GetLabelSelector() string {
	return d.LabelSelector
}

// IsSingleResource implements Discovery.IsSingleResource
func (d *DiscoveryConfig) IsSingleResource() bool {
	return d.ByName != ""
}

// DiscoverResources discovers Kubernetes resources based on the Discovery configuration.
//
// If Discovery.IsSingleResource() is true, it fetches a single resource by name.
// Otherwise, it lists resources matching the label selector.
//
// Example:
//
//	discovery := &k8s_client.DiscoveryConfig{
//	    Namespace:     "default",
//	    LabelSelector: "app=myapp",
//	}
//	list, err := client.DiscoverResources(ctx, gvk, discovery)
func (c *Client) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery Discovery) (*unstructured.UnstructuredList, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	if discovery == nil {
		return list, nil
	}

	if discovery.IsSingleResource() {
		// Single resource by name
		c.log.Infof("Discovering single resource: %s/%s (namespace: %s)",
			gvk.Kind, discovery.GetName(), discovery.GetNamespace())

		obj, err := c.GetResource(ctx, gvk, discovery.GetNamespace(), discovery.GetName())
		if err != nil {
			return list, err
		}

		// Wrap single resource in a list for consistent return type
		list.Items = []unstructured.Unstructured{*obj}
		return list, nil
	}

	// List resources by selector
	return c.ListResources(ctx, gvk, discovery.GetNamespace(), discovery.GetLabelSelector())
}

// BuildLabelSelector converts a map of labels to a selector string.
// Example: {"app": "myapp", "env": "prod"} -> "app=myapp,env=prod"
func BuildLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	return strings.Join(pairs, ",")
}

