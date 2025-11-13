package k8sclient

import (
	"context"

	pkgerrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceManager manages Kubernetes resource lifecycle and tracking
type ResourceManager struct {
	client  *Client
	tracker *ResourceTracker
	log     logger.Logger
}

// NewResourceManager creates a new resource manager
func NewResourceManager(client *Client, log logger.Logger) *ResourceManager {
	return &ResourceManager{
		client:  client,
		tracker: NewResourceTracker(client, log),
		log:     log,
	}
}

// CreateResourceFromTemplate creates a Kubernetes resource from a template
// If track config is provided, the resource will be tracked for status evaluation
func (m *ResourceManager) CreateResourceFromTemplate(ctx context.Context, tmpl ResourceTemplate, variables map[string]interface{}) (*unstructured.Unstructured, error) {
	m.log.V(2).Info("Creating resource from template")

	// Render and parse the template
	obj, err := RenderAndParseResource(tmpl.Template, variables)
	if err != nil {
		return nil, pkgerrors.KubernetesError("failed to render and parse template: %v", err)
	}

	// Create the resource
	created, err := m.client.CreateResource(ctx, obj)
	if err != nil {
		return nil, pkgerrors.KubernetesError("failed to create resource: %v", err)
	}

	// Track the resource if track config is provided
	if tmpl.Track != nil && tmpl.Track.As != "" {
		m.tracker.TrackResource(tmpl.Track.As, created)
	}

	return created, nil
}

// CreateOrUpdateResource creates a resource or updates it if it already exists
func (m *ResourceManager) CreateOrUpdateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	gvk := obj.GroupVersionKind()
	namespace := obj.GetNamespace()
	name := obj.GetName()

	m.log.V(2).Infof("Creating or updating resource: %s/%s (namespace: %s)", gvk.Kind, name, namespace)

	// Try to get the existing resource
	existing, err := m.client.GetResource(ctx, gvk, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Resource doesn't exist, create it
			return m.client.CreateResource(ctx, obj)
		}
		return nil, pkgerrors.KubernetesError("failed to check if resource exists: %v", err)
	}

	// Resource exists, update it
	obj.SetResourceVersion(existing.GetResourceVersion())
	return m.client.UpdateResource(ctx, obj)
}

// ResourceExists checks if a resource exists using discovery config
func (m *ResourceManager) ResourceExists(ctx context.Context, gvk schema.GroupVersionKind, discovery DiscoveryConfig, variables map[string]interface{}) (bool, *unstructured.Unstructured, error) {
	// Render namespace from discovery config (supports template variables)
	namespace := ""
	if discovery.Namespace != "" {
		renderedNamespace, err := RenderTemplate(discovery.Namespace, variables)
		if err != nil {
			return false, nil, pkgerrors.KubernetesError("failed to render discovery namespace: %v", err)
		}
		namespace = renderedNamespace
	}

	m.log.V(2).Infof("Checking if resource exists: %s (namespace: %s)", gvk.Kind, namespace)

	if discovery.ByName != nil {
		// Check by name
		name, err := RenderDiscoveryName(discovery.ByName.Name, variables)
		if err != nil {
			return false, nil, pkgerrors.KubernetesError("failed to render discovery name: %v", err)
		}

		resource, err := m.client.GetResource(ctx, gvk, namespace, name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil, nil
			}
			return false, nil, pkgerrors.KubernetesError("failed to get resource: %v", err)
		}

		return true, resource, nil

	} else if discovery.BySelectors != nil {
		// Check by selectors
		renderedSelectors, err := RenderDiscoverySelectors(discovery.BySelectors.LabelSelector, variables)
		if err != nil {
			return false, nil, pkgerrors.KubernetesError("failed to render discovery selectors: %v", err)
		}

		labelSelector := BuildLabelSelector(renderedSelectors)
		list, err := m.client.ListResources(ctx, gvk, namespace, labelSelector)
		if err != nil {
			return false, nil, pkgerrors.KubernetesError("failed to list resources: %v", err)
		}

		if len(list.Items) == 0 {
			return false, nil, nil
		}

		// Return the first matching resource
		return true, &list.Items[0], nil
	}

	return false, nil, pkgerrors.KubernetesError("no discovery method specified")
}

// DiscoverAndTrack discovers resources based on track configuration and adds them to tracker.
// This function handles all template rendering and passes rendered values to the tracker.
func (m *ResourceManager) DiscoverAndTrack(ctx context.Context, gvk schema.GroupVersionKind, track TrackConfig, variables map[string]interface{}) error {
	if track.As == "" {
		return pkgerrors.KubernetesError("track alias 'as' is required")
	}

	discovery := track.Discovery

	// Render namespace from discovery config (if set)
	namespace := ""
	if discovery.Namespace != "" {
		renderedNamespace, err := RenderTemplate(discovery.Namespace, variables)
		if err != nil {
			return pkgerrors.KubernetesError("failed to render discovery namespace for %s: %v", track.As, err)
		}
		namespace = renderedNamespace
	}

	// Discover by name
	if discovery.ByName != nil {
		renderedName, err := RenderDiscoveryName(discovery.ByName.Name, variables)
		if err != nil {
			return pkgerrors.KubernetesError("failed to render discovery name for %s: %v", track.As, err)
		}

		// Call tracker with already-rendered values
		return m.tracker.DiscoverAndTrackByName(ctx, track.As, gvk, namespace, renderedName)
	}

	// Discover by selectors
	if discovery.BySelectors != nil {
		renderedSelectors, err := RenderDiscoverySelectors(discovery.BySelectors.LabelSelector, variables)
		if err != nil {
			return pkgerrors.KubernetesError("failed to render discovery selectors for %s: %v", track.As, err)
		}

		labelSelector := BuildLabelSelector(renderedSelectors)

		// Call tracker with already-rendered values
		return m.tracker.DiscoverAndTrackBySelectors(ctx, track.As, gvk, namespace, labelSelector)
	}

	return pkgerrors.KubernetesError("no discovery method specified for %s", track.As)
}

// GetTracker returns the resource tracker
func (m *ResourceManager) GetTracker() *ResourceTracker {
	return m.tracker
}

// GetTrackedResourcesAsVariables returns tracked resources as a variables map
// suitable for expression evaluation
func (m *ResourceManager) GetTrackedResourcesAsVariables() map[string]interface{} {
	return m.tracker.BuildVariablesMap()
}

// RefreshTrackedResources refreshes all tracked resources from the Kubernetes API
func (m *ResourceManager) RefreshTrackedResources(ctx context.Context) error {
	return m.tracker.RefreshAllResources(ctx)
}

// ClearTrackedResources removes all tracked resources
func (m *ResourceManager) ClearTrackedResources() {
	m.tracker.Clear()
}

// GetClient returns the underlying Kubernetes client
func (m *ResourceManager) GetClient() *Client {
	return m.client
}

