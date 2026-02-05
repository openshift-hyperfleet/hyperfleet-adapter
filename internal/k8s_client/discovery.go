package k8s_client

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DiscoveryConfig is an alias to manifest.DiscoveryConfig for convenience.
// This allows k8s_client users to use k8s_client.DiscoveryConfig without importing manifest package.
type DiscoveryConfig = manifest.DiscoveryConfig

// BuildLabelSelector is an alias to manifest.BuildLabelSelector for convenience.
// Converts a map of labels to a selector string.
// Example: {"env": "prod", "app": "myapp"} -> "app=myapp,env=prod"
var BuildLabelSelector = manifest.BuildLabelSelector

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
func (c *Client) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery manifest.Discovery) (*unstructured.UnstructuredList, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	if discovery == nil {
		return list, nil
	}

	if discovery.IsSingleResource() {
		// Single resource by name
		c.log.Infof(ctx, "Discovering single resource: %s/%s (namespace: %s)",
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
