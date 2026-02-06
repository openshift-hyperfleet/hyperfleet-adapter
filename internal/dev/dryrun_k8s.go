package dev

import (
	"context"
	"fmt"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DryRunK8sClient implements k8s_client.K8sClient for dry-run mode.
// It logs all operations without executing them and tracks what would happen.
type DryRunK8sClient struct {
	mu sync.Mutex
	// Operations records all operations that would be performed
	Operations []K8sOperation
	// ExistingResources can be pre-populated with resources that should "exist"
	ExistingResources map[string]*unstructured.Unstructured
	// SecretValues can be pre-populated with secret values for extraction
	SecretValues map[string]string
	// ConfigMapValues can be pre-populated with configmap values for extraction
	ConfigMapValues map[string]string
}

// K8sOperation represents a Kubernetes operation that would be performed
type K8sOperation struct {
	// Type is the operation type (get, create, update, delete, discover)
	Type string
	// GVK is the GroupVersionKind of the resource
	GVK schema.GroupVersionKind
	// Namespace is the resource namespace
	Namespace string
	// Name is the resource name
	Name string
	// Resource is the resource object (for create/update)
	Resource *unstructured.Unstructured
	// Manifest is the YAML representation
	Manifest string
}

// NewDryRunK8sClient creates a new DryRunK8sClient
func NewDryRunK8sClient() *DryRunK8sClient {
	return &DryRunK8sClient{
		Operations:        make([]K8sOperation, 0),
		ExistingResources: make(map[string]*unstructured.Unstructured),
		SecretValues:      make(map[string]string),
		ConfigMapValues:   make(map[string]string),
	}
}

// resourceKey generates a unique key for a resource
func resourceKey(gvk schema.GroupVersionKind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s", gvk.Group, gvk.Kind, namespace, name)
}

// GetResource retrieves a resource (returns from ExistingResources or nil)
func (c *DryRunK8sClient) GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Operations = append(c.Operations, K8sOperation{
		Type:      "get",
		GVK:       gvk,
		Namespace: namespace,
		Name:      name,
	})

	key := resourceKey(gvk, namespace, name)
	if res, exists := c.ExistingResources[key]; exists {
		return res.DeepCopy(), nil
	}

	// Return "not found" by default
	return nil, fmt.Errorf("resource %s not found (dry-run)", key)
}

// CreateResource records a create operation
func (c *DryRunK8sClient) CreateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	manifest, _ := yaml.Marshal(obj.Object)

	c.Operations = append(c.Operations, K8sOperation{
		Type:      "create",
		GVK:       obj.GroupVersionKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Resource:  obj.DeepCopy(),
		Manifest:  string(manifest),
	})

	// Return a copy with some "server-generated" fields
	result := obj.DeepCopy()
	result.SetResourceVersion("1")
	result.SetUID("dry-run-uid")

	// Store it so subsequent gets can find it
	key := resourceKey(obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
	c.ExistingResources[key] = result

	return result, nil
}

// UpdateResource records an update operation
func (c *DryRunK8sClient) UpdateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	manifest, _ := yaml.Marshal(obj.Object)

	c.Operations = append(c.Operations, K8sOperation{
		Type:      "update",
		GVK:       obj.GroupVersionKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Resource:  obj.DeepCopy(),
		Manifest:  string(manifest),
	})

	// Return a copy with incremented resource version
	result := obj.DeepCopy()
	result.SetResourceVersion("2")

	// Update the stored resource
	key := resourceKey(obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
	c.ExistingResources[key] = result

	return result, nil
}

// DeleteResource records a delete operation
func (c *DryRunK8sClient) DeleteResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Operations = append(c.Operations, K8sOperation{
		Type:      "delete",
		GVK:       gvk,
		Namespace: namespace,
		Name:      name,
	})

	// Remove from existing resources
	key := resourceKey(gvk, namespace, name)
	delete(c.ExistingResources, key)

	return nil
}

// DiscoverResources records a discover operation
func (c *DryRunK8sClient) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery k8s_client.Discovery) (*unstructured.UnstructuredList, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Operations = append(c.Operations, K8sOperation{
		Type:      "discover",
		GVK:       gvk,
		Namespace: discovery.GetNamespace(),
		Name:      discovery.GetName(),
	})

	// Return an empty list
	return &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{},
	}, nil
}

// ExtractFromSecret returns a pre-configured value or empty string
func (c *DryRunK8sClient) ExtractFromSecret(ctx context.Context, path string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if val, exists := c.SecretValues[path]; exists {
		return val, nil
	}
	return "", fmt.Errorf("secret %s not found (dry-run)", path)
}

// ExtractFromConfigMap returns a pre-configured value or empty string
func (c *DryRunK8sClient) ExtractFromConfigMap(ctx context.Context, path string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if val, exists := c.ConfigMapValues[path]; exists {
		return val, nil
	}
	return "", fmt.Errorf("configmap %s not found (dry-run)", path)
}

// GetOperations returns all recorded operations
func (c *DryRunK8sClient) GetOperations() []K8sOperation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]K8sOperation{}, c.Operations...)
}

// GetManifests returns all recorded manifests
func (c *DryRunK8sClient) GetManifests() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	manifests := make(map[string]string)
	for _, op := range c.Operations {
		if op.Type == "create" || op.Type == "update" {
			key := fmt.Sprintf("%s/%s", op.Namespace, op.Name)
			if op.Namespace == "" {
				key = op.Name
			}
			manifests[key] = op.Manifest
		}
	}
	return manifests
}

// Ensure DryRunK8sClient implements K8sClient
var _ k8s_client.K8sClient = (*DryRunK8sClient)(nil)
