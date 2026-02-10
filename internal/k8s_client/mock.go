package k8s_client

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// MockK8sClient implements K8sClient for testing.
// It stores resources in memory and allows configuring mock responses.
type MockK8sClient struct {
	// Resources stores created/updated resources by "namespace/name" key
	Resources map[string]*unstructured.Unstructured

	// Mock responses - set these to control behavior
	GetResourceResult    *unstructured.Unstructured
	GetResourceError     error
	CreateResourceResult *unstructured.Unstructured
	CreateResourceError  error
	UpdateResourceResult *unstructured.Unstructured
	UpdateResourceError  error
	DeleteResourceError  error
	DiscoverResult       *unstructured.UnstructuredList
	DiscoverError        error
	ApplyResult          *transport_client.ApplyResult
	ApplyError           error
	ApplyResourcesResult *transport_client.ApplyResourcesResult
	ApplyResourcesError  error
}

// NewMockK8sClient creates a new mock K8s client for testing
func NewMockK8sClient() *MockK8sClient {
	return &MockK8sClient{
		Resources: make(map[string]*unstructured.Unstructured),
	}
}

// GetResource implements K8sClient.GetResource
func (m *MockK8sClient) GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	if m.GetResourceError != nil {
		return nil, m.GetResourceError
	}
	if m.GetResourceResult != nil {
		return m.GetResourceResult, nil
	}
	key := namespace + "/" + name
	if res, ok := m.Resources[key]; ok {
		return res, nil
	}
	gr := schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind + "s"}
	return nil, apierrors.NewNotFound(gr, name)
}

// CreateResource implements K8sClient.CreateResource
func (m *MockK8sClient) CreateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if m.CreateResourceError != nil {
		return nil, m.CreateResourceError
	}
	if m.CreateResourceResult != nil {
		return m.CreateResourceResult, nil
	}
	key := obj.GetNamespace() + "/" + obj.GetName()
	m.Resources[key] = obj.DeepCopy()
	return obj, nil
}

// UpdateResource implements K8sClient.UpdateResource
func (m *MockK8sClient) UpdateResource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if m.UpdateResourceError != nil {
		return nil, m.UpdateResourceError
	}
	if m.UpdateResourceResult != nil {
		return m.UpdateResourceResult, nil
	}
	key := obj.GetNamespace() + "/" + obj.GetName()
	m.Resources[key] = obj.DeepCopy()
	return obj, nil
}

// DeleteResource implements K8sClient.DeleteResource
func (m *MockK8sClient) DeleteResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) error {
	if m.DeleteResourceError != nil {
		return m.DeleteResourceError
	}
	key := namespace + "/" + name
	delete(m.Resources, key)
	return nil
}

// DiscoverResources implements K8sClient.DiscoverResources
func (m *MockK8sClient) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery Discovery) (*unstructured.UnstructuredList, error) {
	if m.DiscoverError != nil {
		return nil, m.DiscoverError
	}
	if m.DiscoverResult != nil {
		return m.DiscoverResult, nil
	}
	return &unstructured.UnstructuredList{}, nil
}

// ApplyResource implements K8sClient.ApplyResource
func (m *MockK8sClient) ApplyResource(ctx context.Context, resource transport_client.ResourceToApply, opts transport_client.ApplyOptions) (*transport_client.ApplyResult, error) {
	if m.ApplyError != nil {
		return nil, m.ApplyError
	}
	if m.ApplyResult != nil {
		return m.ApplyResult, nil
	}

	// Default behavior: determine operation from generation comparison
	existing := resource.ExistingResource
	newManifest := resource.Manifest

	manifestGen := manifest.GetGenerationFromUnstructured(newManifest)
	var existingGen int64
	if existing != nil {
		existingGen = manifest.GetGenerationFromUnstructured(existing)
	}

	decision := manifest.CompareGenerations(manifestGen, existingGen, existing != nil)
	operation := string(decision.Operation)
	if decision.Operation == manifest.OperationUpdate && opts.RecreateOnChange {
		operation = string(manifest.OperationRecreate)
	}

	switch manifest.Operation(operation) {
	case manifest.OperationCreate:
		obj, err := m.CreateResource(ctx, newManifest)
		return &transport_client.ApplyResult{Operation: operation, Reason: decision.Reason, Resource: obj, Error: err}, err
	case manifest.OperationUpdate:
		if existing != nil {
			newManifest.SetResourceVersion(existing.GetResourceVersion())
			newManifest.SetUID(existing.GetUID())
		}
		obj, err := m.UpdateResource(ctx, newManifest)
		return &transport_client.ApplyResult{Operation: operation, Reason: decision.Reason, Resource: obj, Error: err}, err
	case manifest.OperationRecreate:
		if existing != nil {
			gvk := existing.GroupVersionKind()
			_ = m.DeleteResource(ctx, gvk, existing.GetNamespace(), existing.GetName()) //nolint:errcheck // mock: best-effort delete before recreate
		}
		obj, err := m.CreateResource(ctx, newManifest)
		return &transport_client.ApplyResult{Operation: operation, Reason: decision.Reason, Resource: obj, Error: err}, err
	case manifest.OperationSkip:
		return &transport_client.ApplyResult{Operation: operation, Reason: decision.Reason, Resource: existing}, nil
	}

	return &transport_client.ApplyResult{Operation: operation, Reason: decision.Reason, Resource: newManifest}, nil
}

// ApplyResources implements TransportClient.ApplyResources
func (m *MockK8sClient) ApplyResources(ctx context.Context, resources []transport_client.ResourceToApply, opts transport_client.ApplyOptions) (*transport_client.ApplyResourcesResult, error) {
	if m.ApplyResourcesError != nil {
		return nil, m.ApplyResourcesError
	}
	if m.ApplyResourcesResult != nil {
		return m.ApplyResourcesResult, nil
	}

	results := &transport_client.ApplyResourcesResult{
		Results: make([]transport_client.ApplyResult, 0, len(resources)),
	}
	for _, res := range resources {
		result, err := m.ApplyResource(ctx, res, opts)
		if result != nil {
			results.Results = append(results.Results, *result)
		}
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// Ensure MockK8sClient implements K8sClient
var _ K8sClient = (*MockK8sClient)(nil)
