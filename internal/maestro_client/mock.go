package maestro_client

import (
	"context"
	"fmt"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	workv1 "open-cluster-management.io/api/work/v1"
)

// MockMaestroClient provides a mock implementation of ManifestWorkClient for unit testing.
// It tracks all calls made and allows configuring responses.
type MockMaestroClient struct {
	mu sync.Mutex

	// ApplyManifestWorkResult is returned from ApplyManifestWork when ApplyManifestWorkError is nil
	ApplyManifestWorkResult *workv1.ManifestWork

	// ApplyManifestWorkError is returned from ApplyManifestWork when set
	ApplyManifestWorkError error

	// AppliedWorks tracks all ManifestWorks passed to ApplyManifestWork
	AppliedWorks []*workv1.ManifestWork

	// ApplyManifestWorkConsumers tracks the consumer names passed to ApplyManifestWork
	ApplyManifestWorkConsumers []string

	// CreateManifestWorkResult is returned from CreateManifestWork when CreateManifestWorkError is nil
	CreateManifestWorkResult *workv1.ManifestWork

	// CreateManifestWorkError is returned from CreateManifestWork when set
	CreateManifestWorkError error

	// CreatedWorks tracks all ManifestWorks passed to CreateManifestWork
	CreatedWorks []*workv1.ManifestWork

	// GetManifestWorkResult is returned from GetManifestWork when GetManifestWorkError is nil
	GetManifestWorkResult *workv1.ManifestWork

	// GetManifestWorkError is returned from GetManifestWork when set
	GetManifestWorkError error

	// ListManifestWorksResult is returned from ListManifestWorks when ListManifestWorksError is nil
	ListManifestWorksResult *workv1.ManifestWorkList

	// ListManifestWorksError is returned from ListManifestWorks when set
	ListManifestWorksError error

	// DeleteManifestWorkError is returned from DeleteManifestWork when set
	DeleteManifestWorkError error

	// DeletedWorks tracks all (consumer, workName) pairs passed to DeleteManifestWork
	DeletedWorks []DeletedWorkRef

	// PatchManifestWorkResult is returned from PatchManifestWork when PatchManifestWorkError is nil
	PatchManifestWorkResult *workv1.ManifestWork

	// PatchManifestWorkError is returned from PatchManifestWork when set
	PatchManifestWorkError error

	// PatchedWorks tracks all patch operations
	PatchedWorks []PatchedWorkRef
}

// DeletedWorkRef tracks a delete operation
type DeletedWorkRef struct {
	ConsumerName string
	WorkName     string
}

// PatchedWorkRef tracks a patch operation
type PatchedWorkRef struct {
	ConsumerName string
	WorkName     string
	PatchData    []byte
}

// NewMockMaestroClient creates a new MockMaestroClient with default settings.
// By default, ApplyManifestWork returns the input work with ResourceVersion "1".
func NewMockMaestroClient() *MockMaestroClient {
	return &MockMaestroClient{
		AppliedWorks:               make([]*workv1.ManifestWork, 0),
		ApplyManifestWorkConsumers: make([]string, 0),
		CreatedWorks:               make([]*workv1.ManifestWork, 0),
		DeletedWorks:               make([]DeletedWorkRef, 0),
		PatchedWorks:               make([]PatchedWorkRef, 0),
	}
}

// Ensure MockMaestroClient implements ManifestWorkClient
var _ ManifestWorkClient = (*MockMaestroClient)(nil)

// Ensure MockMaestroClient implements TransportClient
var _ transport_client.TransportClient = (*MockMaestroClient)(nil)

// ApplyManifestWork creates or updates a ManifestWork (upsert operation)
func (m *MockMaestroClient) ApplyManifestWork(ctx context.Context, consumerName string, work *workv1.ManifestWork) (*workv1.ManifestWork, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.AppliedWorks = append(m.AppliedWorks, work.DeepCopy())
	m.ApplyManifestWorkConsumers = append(m.ApplyManifestWorkConsumers, consumerName)

	if m.ApplyManifestWorkError != nil {
		return nil, m.ApplyManifestWorkError
	}

	if m.ApplyManifestWorkResult != nil {
		return m.ApplyManifestWorkResult.DeepCopy(), nil
	}

	// Default: return the work with a resource version
	result := work.DeepCopy()
	result.ResourceVersion = "1"
	return result, nil
}

// CreateManifestWork creates a new ManifestWork for a target cluster (consumer)
func (m *MockMaestroClient) CreateManifestWork(ctx context.Context, consumerName string, work *workv1.ManifestWork) (*workv1.ManifestWork, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreatedWorks = append(m.CreatedWorks, work.DeepCopy())

	if m.CreateManifestWorkError != nil {
		return nil, m.CreateManifestWorkError
	}

	if m.CreateManifestWorkResult != nil {
		return m.CreateManifestWorkResult.DeepCopy(), nil
	}

	// Default: return the work with a resource version
	result := work.DeepCopy()
	result.ResourceVersion = "1"
	return result, nil
}

// GetManifestWork retrieves a ManifestWork by name from a target cluster
func (m *MockMaestroClient) GetManifestWork(ctx context.Context, consumerName string, workName string) (*workv1.ManifestWork, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.GetManifestWorkError != nil {
		return nil, m.GetManifestWorkError
	}

	if m.GetManifestWorkResult != nil {
		return m.GetManifestWorkResult.DeepCopy(), nil
	}

	return nil, nil
}

// DeleteManifestWork deletes a ManifestWork from a target cluster
func (m *MockMaestroClient) DeleteManifestWork(ctx context.Context, consumerName string, workName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeletedWorks = append(m.DeletedWorks, DeletedWorkRef{
		ConsumerName: consumerName,
		WorkName:     workName,
	})

	return m.DeleteManifestWorkError
}

// ListManifestWorks lists all ManifestWorks for a target cluster
func (m *MockMaestroClient) ListManifestWorks(ctx context.Context, consumerName string, labelSelector string) (*workv1.ManifestWorkList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ListManifestWorksError != nil {
		return nil, m.ListManifestWorksError
	}

	if m.ListManifestWorksResult != nil {
		return m.ListManifestWorksResult.DeepCopy(), nil
	}

	return &workv1.ManifestWorkList{}, nil
}

// PatchManifestWork patches an existing ManifestWork using JSON merge patch
func (m *MockMaestroClient) PatchManifestWork(ctx context.Context, consumerName string, workName string, patchData []byte) (*workv1.ManifestWork, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PatchedWorks = append(m.PatchedWorks, PatchedWorkRef{
		ConsumerName: consumerName,
		WorkName:     workName,
		PatchData:    patchData,
	})

	if m.PatchManifestWorkError != nil {
		return nil, m.PatchManifestWorkError
	}

	if m.PatchManifestWorkResult != nil {
		return m.PatchManifestWorkResult.DeepCopy(), nil
	}

	return nil, nil
}

// Reset clears all tracked calls and resets configured responses
func (m *MockMaestroClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ApplyManifestWorkResult = nil
	m.ApplyManifestWorkError = nil
	m.AppliedWorks = make([]*workv1.ManifestWork, 0)
	m.ApplyManifestWorkConsumers = make([]string, 0)
	m.CreateManifestWorkResult = nil
	m.CreateManifestWorkError = nil
	m.CreatedWorks = make([]*workv1.ManifestWork, 0)
	m.GetManifestWorkResult = nil
	m.GetManifestWorkError = nil
	m.ListManifestWorksResult = nil
	m.ListManifestWorksError = nil
	m.DeleteManifestWorkError = nil
	m.DeletedWorks = make([]DeletedWorkRef, 0)
	m.PatchManifestWorkResult = nil
	m.PatchManifestWorkError = nil
	m.PatchedWorks = make([]PatchedWorkRef, 0)
}

// GetAppliedWorks returns a copy of all applied works (thread-safe)
func (m *MockMaestroClient) GetAppliedWorks() []*workv1.ManifestWork {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*workv1.ManifestWork, len(m.AppliedWorks))
	for i, w := range m.AppliedWorks {
		result[i] = w.DeepCopy()
	}
	return result
}

// GetApplyConsumers returns a copy of all consumer names used in ApplyManifestWork (thread-safe)
func (m *MockMaestroClient) GetApplyConsumers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]string, len(m.ApplyManifestWorkConsumers))
	copy(result, m.ApplyManifestWorkConsumers)
	return result
}

// --- TransportClient implementation ---

// ApplyResources implements transport_client.TransportClient.
// It delegates to ApplyManifestWork internally so that existing test assertions
// on GetAppliedWorks() and GetApplyConsumers() continue to work.
func (m *MockMaestroClient) ApplyResources(ctx context.Context, resources []transport_client.ResourceToApply, opts transport_client.ApplyOptions) (*transport_client.ApplyResourcesResult, error) {
	// Extract transport config
	tc := opts.TransportConfig
	if tc == nil {
		return nil, fmt.Errorf("TransportConfig is required for Maestro mock")
	}

	targetCluster, _ := tc["targetCluster"].(string) //nolint:errcheck // type assertion with zero-value default
	if targetCluster == "" {
		return nil, fmt.Errorf("targetCluster is required in TransportConfig")
	}

	// Collect manifests
	manifests := make([]*unstructured.Unstructured, 0, len(resources))
	for _, res := range resources {
		manifests = append(manifests, res.Manifest)
	}

	// Build ManifestWork using the shared helper (from client.go)
	manifestWorkName, _ := tc["manifestWorkName"].(string)                 //nolint:errcheck // optional, zero-value default
	resourceName, _ := tc["resourceName"].(string)                         //nolint:errcheck // optional, zero-value default
	refContent, _ := tc["manifestWorkRefContent"].(map[string]interface{}) //nolint:errcheck // optional, nil default
	params, _ := tc["params"].(map[string]interface{})                     //nolint:errcheck // optional, nil default

	work, err := buildManifestWork(ctx, nil, manifests, manifestWorkName, resourceName, refContent, params)
	if err != nil {
		return nil, fmt.Errorf("failed to build ManifestWork: %w", err)
	}

	// Delegate to ApplyManifestWork so existing test assertions work
	_, err = m.ApplyManifestWork(ctx, targetCluster, work)
	if err != nil {
		return nil, fmt.Errorf("failed to apply ManifestWork: %w", err)
	}

	// Build results
	results := &transport_client.ApplyResourcesResult{
		Results: make([]transport_client.ApplyResult, 0, len(resources)),
	}
	for _, res := range resources {
		results.Results = append(results.Results, transport_client.ApplyResult{
			Operation: string(manifest.OperationCreate),
			Reason:    fmt.Sprintf("ManifestWork applied to cluster %s with %d manifests", targetCluster, len(manifests)),
			Resource:  res.Manifest,
		})
	}

	return results, nil
}

// GetResource implements transport_client.TransportClient.
// Returns NotFound for consistency with the real Maestro client.
func (m *MockMaestroClient) GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	return nil, apierrors.NewNotFound(
		schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind},
		name,
	)
}

// DiscoverResources implements transport_client.TransportClient.
// Returns an empty list for Maestro transport.
func (m *MockMaestroClient) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery transport_client.Discovery) (*unstructured.UnstructuredList, error) {
	return &unstructured.UnstructuredList{}, nil
}
