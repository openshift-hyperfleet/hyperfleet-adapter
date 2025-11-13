package k8sclient

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is the Kubernetes client for managing resources
type Client struct {
	dynamicClient  dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	mapper         meta.RESTMapper
	log            logger.Logger
}

// ClientConfig holds configuration for creating a Kubernetes client
type ClientConfig struct {
	// KubeConfigPath is the path to kubeconfig file
	// Leave empty ("") to use in-cluster ServiceAccount authentication
	// Set to a path for local development or external cluster access
	KubeConfigPath string
	// QPS is the queries per second rate limiter
	QPS float32
	// Burst is the burst rate limiter
	Burst int
}

// NewClient creates a new Kubernetes client with automatic authentication detection
//
// Authentication Methods:
//   1. In-Cluster (ServiceAccount) - When KubeConfigPath is empty ("")
//      - Uses ServiceAccount token mounted at /var/run/secrets/kubernetes.io/serviceaccount/token
//      - Uses CA certificate at /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
//      - Automatically configured when running in a Kubernetes pod
//      - Requires appropriate RBAC permissions for the ServiceAccount
//
//   2. Kubeconfig - When KubeConfigPath is set
//      - Uses the specified kubeconfig file for authentication
//      - Suitable for local development or accessing remote clusters
//
// Example Usage:
//   // For production deployment in K8s cluster (uses ServiceAccount)
//   config := ClientConfig{KubeConfigPath: "", QPS: 100.0, Burst: 200}
//   client, err := NewClient(ctx, config, log)
//
//   // For local development (uses kubeconfig)
//   config := ClientConfig{KubeConfigPath: "/home/user/.kube/config"}
//   client, err := NewClient(ctx, config, log)
func NewClient(ctx context.Context, config ClientConfig, log logger.Logger) (*Client, error) {
	var restConfig *rest.Config
	var err error

	if config.KubeConfigPath == "" {
		// Use in-cluster config with ServiceAccount
		// This reads from:
		// - Token: /var/run/secrets/kubernetes.io/serviceaccount/token
		// - CA Cert: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
		// - Namespace: /var/run/secrets/kubernetes.io/serviceaccount/namespace
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.KubernetesError("failed to create in-cluster config: %v", err)
		}
		log.Info("Using in-cluster Kubernetes configuration (ServiceAccount)")
	} else {
		// Use kubeconfig file for local development or remote access
		restConfig, err = clientcmd.BuildConfigFromFlags("", config.KubeConfigPath)
		if err != nil {
			return nil, errors.KubernetesError("failed to build config from kubeconfig %s: %v", config.KubeConfigPath, err)
		}
		log.Infof("Using kubeconfig from: %s", config.KubeConfigPath)
	}

	return NewClientFromConfig(ctx, restConfig, log)
}

// NewClientFromConfig creates a new Kubernetes client from a rest.Config
// This is useful for testing with envtest or when you already have a rest.Config
//
// Example Usage:
//   // For testing with envtest
//   testEnv := &envtest.Environment{}
//   cfg, _ := testEnv.Start()
//   client, err := NewClientFromConfig(ctx, cfg, log)
func NewClientFromConfig(ctx context.Context, restConfig *rest.Config, log logger.Logger) (*Client, error) {

	// Set default rate limits if not already set
	if restConfig.QPS == 0 {
		restConfig.QPS = 100.0
	}
	if restConfig.Burst == 0 {
		restConfig.Burst = 200
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.KubernetesError("failed to create dynamic client: %v", err)
	}

	// Create discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, errors.KubernetesError("failed to create discovery client: %v", err)
	}

	// Create REST mapper for GVK to GVR conversion
	mapper, err := createRESTMapper(discoveryClient)
	if err != nil {
		return nil, errors.KubernetesError("failed to create REST mapper: %v", err)
	}

	return &Client{
		dynamicClient:  dynamicClient,
		discoveryClient: discoveryClient,
		mapper:         mapper,
		log:            log,
	}, nil
}

// createRESTMapper creates a REST mapper from discovery client
func createRESTMapper(discoveryClient discovery.DiscoveryInterface) (meta.RESTMapper, error) {
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, errors.KubernetesError("failed to get API group resources: %v", err)
	}
	return restmapper.NewDiscoveryRESTMapper(groupResources), nil
}

// getResourceInterface returns the appropriate resource interface for the given GVK
func (c *Client) getResourceInterface(ctx context.Context, gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error) {
	// Get the GVR from GVK using REST mapper
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, errors.KubernetesError("failed to get REST mapping for %s: %v", gvk.String(), err)
	}

	// Get the resource interface
	var resourceInterface dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		// Namespaced resource
		if namespace == "" {
			return nil, errors.KubernetesError("namespace is required for namespaced resource %s", gvk.String())
		}
		resourceInterface = c.dynamicClient.Resource(mapping.Resource).Namespace(namespace)
	} else {
		// Cluster-scoped resource
		resourceInterface = c.dynamicClient.Resource(mapping.Resource)
	}

	return resourceInterface, nil
}

// CreateResource creates a Kubernetes resource from an unstructured object
func (c *Client) CreateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	gvk := obj.GroupVersionKind()
	namespace := obj.GetNamespace()
	name := obj.GetName()

	c.log.Infof("Creating resource: %s/%s (namespace: %s)", gvk.Kind, name, namespace)

	resourceInterface, err := c.getResourceInterface(ctx, gvk, namespace)
	if err != nil {
		return nil, err
	}

	created, err := resourceInterface.Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.KubernetesError("failed to create resource %s/%s (namespace: %s): %v", gvk.Kind, name, namespace, err)
	}

	c.log.Infof("Successfully created resource: %s/%s", gvk.Kind, name)
	return created, nil
}

// GetResource retrieves a Kubernetes resource by name
func (c *Client) GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	c.log.V(2).Infof("Getting resource: %s/%s (namespace: %s)", gvk.Kind, name, namespace)

	resourceInterface, err := c.getResourceInterface(ctx, gvk, namespace)
	if err != nil {
		return nil, err
	}

	resource, err := resourceInterface.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// Don't wrap NotFound errors so callers can check for them
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, errors.KubernetesError("failed to get resource %s/%s (namespace: %s): %v", gvk.Kind, name, namespace, err)
	}

	c.log.V(2).Infof("Successfully retrieved resource: %s/%s", gvk.Kind, name)
	return resource, nil
}

// ListResources lists Kubernetes resources by label selector
func (c *Client) ListResources(ctx context.Context, gvk schema.GroupVersionKind, namespace string, labelSelector string) (*unstructured.UnstructuredList, error) {
	c.log.V(2).Infof("Listing resources: %s (namespace: %s, selector: %s)", gvk.Kind, namespace, labelSelector)

	resourceInterface, err := c.getResourceInterface(ctx, gvk, namespace)
	if err != nil {
		return nil, err
	}

	list, err := resourceInterface.List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, errors.KubernetesError("failed to list resources %s (namespace: %s, selector: %s): %v", gvk.Kind, namespace, labelSelector, err)
	}

	c.log.V(2).Infof("Successfully listed resources: %s (found %d items)", gvk.Kind, len(list.Items))
	return list, nil
}

// UpdateResource updates an existing Kubernetes resource by replacing it entirely.
//
// This performs a full resource replacement - all fields in the provided object
// will replace the existing resource. Any fields not included will be reset to
// their default values. Requires the object to have a valid resourceVersion.
//
// Use UpdateResource when:
//   - You have the complete, current resource (e.g., from GetResource)
//   - You want to replace the entire resource
//   - You're making multiple changes across the object
//
// Use PatchResource instead when:
//   - You only want to modify specific fields
//   - You don't have the current resource
//   - You want to avoid conflicts with concurrent updates
//
// Example:
//   resource, _ := client.GetResource(ctx, gvk, "default", "my-cm")
//   resource.SetLabels(map[string]string{"app": "myapp"})
//   updated, err := client.UpdateResource(ctx, resource)
func (c *Client) UpdateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	gvk := obj.GroupVersionKind()
	namespace := obj.GetNamespace()
	name := obj.GetName()

	c.log.Infof("Updating resource: %s/%s (namespace: %s)", gvk.Kind, name, namespace)

	resourceInterface, err := c.getResourceInterface(ctx, gvk, namespace)
	if err != nil {
		return nil, err
	}

	updated, err := resourceInterface.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.KubernetesError("failed to update resource %s/%s (namespace: %s): %v", gvk.Kind, name, namespace, err)
	}

	c.log.Infof("Successfully updated resource: %s/%s", gvk.Kind, name)
	return updated, nil
}

// DeleteResource deletes a Kubernetes resource
func (c *Client) DeleteResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	c.log.Infof("Deleting resource: %s/%s (namespace: %s)", gvk.Kind, name, namespace)

	resourceInterface, err := c.getResourceInterface(ctx, gvk, namespace)
	if err != nil {
		return err
	}

	err = resourceInterface.Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return errors.KubernetesError("failed to delete resource %s/%s (namespace: %s): %v", gvk.Kind, name, namespace, err)
	}

	c.log.Infof("Successfully deleted resource: %s/%s", gvk.Kind, name)
	return nil
}

// PatchResource applies a patch to an existing Kubernetes resource, modifying only specified fields.
//
// Unlike UpdateResource which replaces the entire resource, PatchResource only modifies
// the fields you specify in the patch. All other fields remain unchanged. This is more
// efficient and safer for concurrent modifications as it doesn't require a resourceVersion.
//
// Supported patch types:
//   - types.StrategicMergePatchType: Kubernetes-aware merge (recommended for K8s resources)
//   - types.MergePatchType: JSON Merge Patch (RFC 7386)
//   - types.JSONPatchType: JSON Patch operations (RFC 6902)
//
// Use PatchResource when:
//   - You only want to modify specific fields (e.g., add a label)
//   - You don't have the complete current resource
//   - Multiple processes might update different fields concurrently
//   - You want better performance (less data transferred)
//
// Use UpdateResource instead when:
//   - You have the full resource and want to replace everything
//   - You're making extensive changes across the entire object
//
// Example (add a label):
//   patch := []byte(`{"metadata":{"labels":{"app":"myapp"}}}`)
//   patched, err := client.PatchResource(ctx, gvk, "default", "my-cm",
//       patch, types.StrategicMergePatchType)
func (c *Client) PatchResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, patchData []byte, patchType types.PatchType) (*unstructured.Unstructured, error) {
	c.log.Infof("Patching resource: %s/%s (namespace: %s)", gvk.Kind, name, namespace)

	resourceInterface, err := c.getResourceInterface(ctx, gvk, namespace)
	if err != nil {
		return nil, err
	}

	patched, err := resourceInterface.Patch(ctx, name, patchType, patchData, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.KubernetesError("failed to patch resource %s/%s (namespace: %s): %v", gvk.Kind, name, namespace, err)
	}

	c.log.Infof("Successfully patched resource: %s/%s", gvk.Kind, name)
	return patched, nil
}

