package k8s_client

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Discovery is an alias for the transport_client.Discovery interface.
type Discovery = transport_client.Discovery

// DiscoveryConfig is an alias for manifest.DiscoveryConfig.
type DiscoveryConfig = manifest.DiscoveryConfig

// DiscoverResources discovers Kubernetes resources based on the Discovery configuration.
//
// If Discovery.IsSingleResource() is true, it fetches a single resource by name.
// Otherwise, it lists resources matching the label selector.
func (c *Client) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery Discovery) (*unstructured.UnstructuredList, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	if discovery == nil {
		return list, nil
	}

	if discovery.IsSingleResource() {
		c.log.Infof(ctx, "Discovering single resource: %s/%s (namespace: %s)",
			gvk.Kind, discovery.GetName(), discovery.GetNamespace())

		obj, err := c.GetResource(ctx, gvk, discovery.GetNamespace(), discovery.GetName())
		if err != nil {
			return list, err
		}

		list.Items = []unstructured.Unstructured{*obj}
		return list, nil
	}

	return c.ListResources(ctx, gvk, discovery.GetNamespace(), discovery.GetLabelSelector())
}

// BuildLabelSelector converts a map of labels to a selector string.
// Keys are sorted alphabetically for deterministic output.
// Example: {"env": "prod", "app": "myapp"} -> "app=myapp,env=prod"
func BuildLabelSelector(labels map[string]string) string {
	return manifest.BuildLabelSelector(labels)
}
