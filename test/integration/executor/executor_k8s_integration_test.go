package executor_integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// k8sTestAPIServer creates a mock API server for K8s integration tests
type k8sTestAPIServer struct {
	server          *httptest.Server
	mu              sync.Mutex
	requests        []k8sTestRequest
	clusterResponse map[string]interface{}
	statusResponses []map[string]interface{}
}

type k8sTestRequest struct {
	Method string
	Path   string
	Body   string
}

func newK8sTestAPIServer(t *testing.T) *k8sTestAPIServer {
	mock := &k8sTestAPIServer{
		requests: make([]k8sTestRequest, 0),
		clusterResponse: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-cluster",
			},
			"spec": map[string]interface{}{
				"region":     "us-east-1",
				"provider":   "aws",
				"vpc_id":     "vpc-12345",
				"node_count": 3,
			},
			"status": map[string]interface{}{
				"phase": "Ready",
			},
		},
		statusResponses: make([]map[string]interface{}, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		var bodyStr string
		if r.Body != nil {
			buf := make([]byte, 1024*1024)
			n, _ := r.Body.Read(buf)
			bodyStr = string(buf[:n])
		}

		mock.requests = append(mock.requests, k8sTestRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   bodyStr,
		})

		t.Logf("Mock API: %s %s", r.Method, r.URL.Path)

		switch {
		case strings.Contains(r.URL.Path, "/clusters/") && strings.HasSuffix(r.URL.Path, "/status"):
			if r.Method == http.MethodPost {
				var statusBody map[string]interface{}
				if err := json.Unmarshal([]byte(bodyStr), &statusBody); err == nil {
					mock.statusResponses = append(mock.statusResponses, statusBody)
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
				return
			}
		case strings.Contains(r.URL.Path, "/clusters/"):
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(mock.clusterResponse)
				return
			}
		}

		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))

	return mock
}

func (m *k8sTestAPIServer) Close() {
	m.server.Close()
}

func (m *k8sTestAPIServer) URL() string {
	return m.server.URL
}

func (m *k8sTestAPIServer) GetStatusResponses() []map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]map[string]interface{}{}, m.statusResponses...)
}

// createK8sTestEvent creates a CloudEvent for K8s integration testing
func createK8sTestEvent(clusterId string) *event.Event {
	evt := event.New()
	evt.SetID("k8s-test-event-" + clusterId)
	evt.SetType("com.redhat.hyperfleet.cluster.provision")
	evt.SetSource("k8s-integration-test")
	evt.SetTime(time.Now())

	eventData := map[string]interface{}{
		"cluster_id":    clusterId,
		"resource_id":   "resource-" + clusterId,
		"resource_type": "cluster",
		"generation":    "gen-001",
		"href":          "/api/v1/clusters/" + clusterId,
	}
	eventDataBytes, _ := json.Marshal(eventData)
	_ = evt.SetData(event.ApplicationJSON, eventDataBytes)

	return &evt
}

// parseAndExecute is a helper that parses CloudEvent data and calls Execute
func parseAndExecute(t *testing.T, exec *executor.Executor, ctx context.Context, evt *event.Event) *executor.ExecutionResult {
	eventData, rawData, err := executor.ParseEventData(evt.Data())
	require.NoError(t, err, "Failed to parse event data")
	if eventData != nil {
		if eventData.OwnedReference != nil {
			ctx = logger.WithResourceType(ctx, eventData.Kind)
			ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
			ctx = logger.WithDynamicResourceID(ctx, eventData.OwnedReference.Kind, eventData.OwnedReference.ID)
		} else if eventData.Kind != "" {
			ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
		}
	}
	return exec.Execute(ctx, rawData)
}

// createK8sTestConfig creates an AdapterConfig with K8s resources using step-based model
func createK8sTestConfig(apiBaseURL, testNamespace string) *config_loader.AdapterConfig {
	return &config_loader.AdapterConfig{
		APIVersion: "hyperfleet.redhat.com/v1alpha1",
		Kind:       "AdapterConfig",
		Metadata: config_loader.Metadata{
			Name:      "k8s-test-adapter",
			Namespace: testNamespace,
		},
		Spec: config_loader.AdapterConfigSpec{
			Adapter: config_loader.AdapterInfo{
				Version: "1.0.0",
			},
			HyperfleetAPI: config_loader.HyperfleetAPIConfig{
				Timeout:       "10s",
				RetryAttempts: 1,
				RetryBackoff:  "constant",
			},
			Steps: []config_loader.Step{
				// Param steps
				{
					Name: "hyperfleetApiBaseUrl",
					Param: &config_loader.ParamStep{
						Source: "env.HYPERFLEET_API_BASE_URL",
					},
				},
				{
					Name: "hyperfleetApiVersion",
					Param: &config_loader.ParamStep{
						Source:  "env.HYPERFLEET_API_VERSION",
						Default: "v1",
					},
				},
				{
					Name: "clusterId",
					Param: &config_loader.ParamStep{
						Source: "event.cluster_id",
					},
				},
				{
					Name: "testNamespace",
					Param: &config_loader.ParamStep{
						Value: testNamespace,
					},
				},
				// API call step with captures
				{
					Name: "clusterStatus",
					APICall: &config_loader.APICallStep{
						Method:  "GET",
						URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterId }}",
						Timeout: "5s",
						Capture: []config_loader.CaptureField{
							{Name: "clusterName", Field: "metadata.name"},
							{Name: "clusterPhase", Field: "status.phase"},
							{Name: "region", Field: "spec.region"},
							{Name: "cloudProvider", Field: "spec.provider"},
						},
					},
				},
				// Resource steps with when clause
				{
					Name: "clusterConfigMap",
					When: `clusterPhase == "Ready" || clusterPhase == "Provisioning" || clusterPhase == "Installing"`,
					Resource: &config_loader.ResourceStep{
						Manifest: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "cluster-config-{{ .clusterId }}",
								"namespace": testNamespace,
								"labels": map[string]interface{}{
									"hyperfleet.io/cluster-id": "{{ .clusterId }}",
									"hyperfleet.io/managed-by": "{{ .metadata.name }}",
									"test":                     "executor-integration",
								},
								"annotations": map[string]interface{}{
									k8s_client.AnnotationGeneration: "1",
								},
							},
							"data": map[string]interface{}{
								"cluster-id":   "{{ .clusterId }}",
								"cluster-name": "{{ .clusterName }}",
								"region":       "{{ .region }}",
								"provider":     "{{ .cloudProvider }}",
								"phase":        "{{ .clusterPhase }}",
							},
						},
						Discovery: &config_loader.DiscoveryConfig{
							Namespace: testNamespace,
							ByName:    "cluster-config-{{ .clusterId }}",
						},
					},
				},
				{
					Name: "clusterSecret",
					When: `clusterPhase == "Ready" || clusterPhase == "Provisioning" || clusterPhase == "Installing"`,
					Resource: &config_loader.ResourceStep{
						Manifest: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Secret",
							"metadata": map[string]interface{}{
								"name":      "cluster-secret-{{ .clusterId }}",
								"namespace": testNamespace,
								"labels": map[string]interface{}{
									"hyperfleet.io/cluster-id": "{{ .clusterId }}",
									"hyperfleet.io/managed-by": "{{ .metadata.name }}",
									"test":                     "executor-integration",
								},
								"annotations": map[string]interface{}{
									k8s_client.AnnotationGeneration: "1",
								},
							},
							"type": "Opaque",
							"stringData": map[string]interface{}{
								"cluster-id": "{{ .clusterId }}",
								"api-token":  "test-token-{{ .clusterId }}",
							},
						},
						Discovery: &config_loader.DiscoveryConfig{
							Namespace: testNamespace,
							ByName:    "cluster-secret-{{ .clusterId }}",
						},
					},
				},
				// Payload step
				{
					Name: "clusterStatusPayload",
					Payload: map[string]interface{}{
						"conditions": map[string]interface{}{
							"applied": map[string]interface{}{
								"status": map[string]interface{}{
									"expression": `adapter.executionStatus == "success"`,
								},
								"reason": map[string]interface{}{
									"expression": `adapter.?errorReason.orValue("ResourcesCreated")`,
								},
								"message": map[string]interface{}{
									"expression": `adapter.?errorMessage.orValue("ConfigMap and Secret created successfully")`,
								},
							},
						},
						"clusterId": map[string]interface{}{
							"value": "{{ .clusterId }}",
						},
						"resourcesCreated": map[string]interface{}{
							"value": "2",
						},
					},
				},
				// API call step for reporting status
				{
					Name: "reportClusterStatus",
					APICall: &config_loader.APICallStep{
						Method:  "POST",
						URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterId }}/status",
						Body:    "{{ .clusterStatusPayload }}",
						Timeout: "5s",
					},
				},
			},
		},
	}
}

// TestExecutor_K8s_CreateResources tests the full flow with real K8s resource creation
func TestExecutor_K8s_CreateResources(t *testing.T) {
	// Setup K8s test environment
	k8sEnv := SetupK8sTestEnv(t)
	defer k8sEnv.Cleanup(t)

	// Create test namespace
	testNamespace := fmt.Sprintf("executor-test-%d", time.Now().Unix())
	k8sEnv.CreateTestNamespace(t, testNamespace)
	defer k8sEnv.CleanupTestNamespace(t, testNamespace)

	// Setup mock API server
	mockAPI := newK8sTestAPIServer(t)
	defer mockAPI.Close()

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Create config with K8s resources
	config := createK8sTestConfig(mockAPI.URL(), testNamespace)
	apiClient, err := hyperfleet_api.NewClient(testLog(),
		hyperfleet_api.WithTimeout(10*time.Second),
		hyperfleet_api.WithRetryAttempts(1),
	)
	require.NoError(t, err)

	// Create executor with real K8s client
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	// Create test event
	clusterId := fmt.Sprintf("cluster-%d", time.Now().UnixNano())
	evt := createK8sTestEvent(clusterId)

	// Execute
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := parseAndExecute(t, exec, ctx, evt)

	// Verify execution succeeded
	if result.Status != executor.StatusSuccess {
		t.Fatalf("Expected success status, got %s: errors=%v", result.Status, result.Errors)
	}

	t.Logf("Execution completed successfully")

	// Verify step results - find resource steps
	cmStepResult := result.GetStepResult("clusterConfigMap")
	require.NotNil(t, cmStepResult, "Expected clusterConfigMap step result")
	assert.False(t, cmStepResult.Skipped, "ConfigMap step should not be skipped")
	assert.Nil(t, cmStepResult.Error, "ConfigMap step should not have error")
	t.Logf("ConfigMap step completed: skipped=%v", cmStepResult.Skipped)

	secretStepResult := result.GetStepResult("clusterSecret")
	require.NotNil(t, secretStepResult, "Expected clusterSecret step result")
	assert.False(t, secretStepResult.Skipped, "Secret step should not be skipped")
	assert.Nil(t, secretStepResult.Error, "Secret step should not have error")
	t.Logf("Secret step completed: skipped=%v", secretStepResult.Skipped)

	// Verify ConfigMap exists in K8s
	cmGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	cmName := fmt.Sprintf("cluster-config-%s", clusterId)
	cm, err := k8sEnv.Client.GetResource(ctx, cmGVK, testNamespace, cmName)
	require.NoError(t, err, "ConfigMap should exist in K8s")
	assert.Equal(t, cmName, cm.GetName())

	// Verify ConfigMap data
	cmData, found, err := unstructured.NestedStringMap(cm.Object, "data")
	require.NoError(t, err)
	require.True(t, found, "ConfigMap should have data")
	assert.Equal(t, clusterId, cmData["cluster-id"])
	assert.Equal(t, "test-cluster", cmData["cluster-name"])
	assert.Equal(t, "us-east-1", cmData["region"])
	assert.Equal(t, "aws", cmData["provider"])
	assert.Equal(t, "Ready", cmData["phase"])
	t.Logf("ConfigMap data verified: %+v", cmData)

	// Verify ConfigMap labels
	cmLabels := cm.GetLabels()
	assert.Equal(t, clusterId, cmLabels["hyperfleet.io/cluster-id"])
	assert.Equal(t, "k8s-test-adapter", cmLabels["hyperfleet.io/managed-by"])

	// Verify Secret exists in K8s
	secretGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	secretName := fmt.Sprintf("cluster-secret-%s", clusterId)
	secret, err := k8sEnv.Client.GetResource(ctx, secretGVK, testNamespace, secretName)
	require.NoError(t, err, "Secret should exist in K8s")
	assert.Equal(t, secretName, secret.GetName())
	t.Logf("Secret verified: %s", secretName)

	// Verify post action reported status with correct template expression values
	statusResponses := mockAPI.GetStatusResponses()
	require.Len(t, statusResponses, 1, "Should have 1 status response")
	status := statusResponses[0]
	t.Logf("Status reported: %+v", status)

	if conditions, ok := status["conditions"].(map[string]interface{}); ok {
		if applied, ok := conditions["applied"].(map[string]interface{}); ok {
			// Status should be true (adapter.executionStatus == "success")
			assert.Equal(t, true, applied["status"], "Applied status should be true")

			// Reason should be "ResourcesCreated" (default, no adapter.errorReason)
			assert.Equal(t, "ResourcesCreated", applied["reason"], "Should use default reason for success")

			// Message should be success message (default, no adapter.errorMessage)
			if message, ok := applied["message"].(string); ok {
				assert.Equal(t, "ConfigMap and Secret created successfully", message, "Should use default success message")
			}
		}
	}
}

// TestExecutor_K8s_UpdateExistingResource tests updating an existing resource
func TestExecutor_K8s_UpdateExistingResource(t *testing.T) {
	k8sEnv := SetupK8sTestEnv(t)
	defer k8sEnv.Cleanup(t)

	testNamespace := fmt.Sprintf("executor-update-%d", time.Now().Unix())
	k8sEnv.CreateTestNamespace(t, testNamespace)
	defer k8sEnv.CleanupTestNamespace(t, testNamespace)

	mockAPI := newK8sTestAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	clusterId := fmt.Sprintf("update-cluster-%d", time.Now().UnixNano())

	// Pre-create the ConfigMap with an older generation
	existingCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("cluster-config-%s", clusterId),
				"namespace": testNamespace,
				"labels": map[string]interface{}{
					"hyperfleet.io/cluster-id": clusterId,
					"hyperfleet.io/managed-by": "k8s-test-adapter",
					"test":                     "executor-integration",
				},
				"annotations": map[string]interface{}{
					k8s_client.AnnotationGeneration: "0", // Older generation
				},
			},
			"data": map[string]interface{}{
				"cluster-id": clusterId,
				"phase":      "Provisioning", // Old value
			},
		},
	}
	existingCM.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})

	ctx := context.Background()
	_, err := k8sEnv.Client.CreateResource(ctx, existingCM)
	require.NoError(t, err, "Failed to pre-create ConfigMap")
	t.Logf("Pre-created ConfigMap with phase=Provisioning")

	// Create executor - use only ConfigMap step
	config := createK8sTestConfig(mockAPI.URL(), testNamespace)
	// Keep only steps up to and including clusterConfigMap, plus payload and report
	filteredSteps := []config_loader.Step{}
	for _, step := range config.Spec.Steps {
		if step.Name == "clusterSecret" {
			continue // Skip secret step
		}
		filteredSteps = append(filteredSteps, step)
	}
	config.Spec.Steps = filteredSteps

	apiClient, err := hyperfleet_api.NewClient(testLog())
	require.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	// Execute - should update existing resource
	evt := createK8sTestEvent(clusterId)
	result := parseAndExecute(t, exec, ctx, evt)

	require.Equal(t, executor.StatusSuccess, result.Status, "Execution should succeed: errors=%v", result.Errors)

	// Verify ConfigMap was updated with new data
	cmGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	cmName := fmt.Sprintf("cluster-config-%s", clusterId)
	updatedCM, err := k8sEnv.Client.GetResource(ctx, cmGVK, testNamespace, cmName)
	require.NoError(t, err)

	cmData, _, _ := unstructured.NestedStringMap(updatedCM.Object, "data")
	assert.Equal(t, "Ready", cmData["phase"], "Phase should be updated to Ready")
	assert.Equal(t, "test-cluster", cmData["cluster-name"], "Should have new cluster-name field")
	t.Logf("Updated ConfigMap data: %+v", cmData)

	// Verify status payload was built and sent with correct template expression values
	statusResponses := mockAPI.GetStatusResponses()
	require.Len(t, statusResponses, 1, "Should have reported status")
	status := statusResponses[0]
	t.Logf("Status reported after update: %+v", status)

	// Verify the status payload contains success values from template expressions
	if conditions, ok := status["conditions"].(map[string]interface{}); ok {
		if applied, ok := conditions["applied"].(map[string]interface{}); ok {
			// Status should be true (adapter.executionStatus == "success")
			assert.Equal(t, true, applied["status"], "Applied status should be true for successful update")
		}
	}
}

// TestExecutor_K8s_DiscoveryByLabels tests resource discovery using label selectors
func TestExecutor_K8s_DiscoveryByLabels(t *testing.T) {
	k8sEnv := SetupK8sTestEnv(t)
	defer k8sEnv.Cleanup(t)

	testNamespace := fmt.Sprintf("executor-discovery-%d", time.Now().Unix())
	k8sEnv.CreateTestNamespace(t, testNamespace)
	defer k8sEnv.CleanupTestNamespace(t, testNamespace)

	mockAPI := newK8sTestAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	clusterId := fmt.Sprintf("discovery-cluster-%d", time.Now().UnixNano())

	// Create config with label-based discovery
	config := &config_loader.AdapterConfig{
		APIVersion: "hyperfleet.redhat.com/v1alpha1",
		Kind:       "AdapterConfig",
		Metadata: config_loader.Metadata{
			Name:      "discovery-test",
			Namespace: testNamespace,
		},
		Spec: config_loader.AdapterConfigSpec{
			Adapter:       config_loader.AdapterInfo{Version: "1.0.0"},
			HyperfleetAPI: config_loader.HyperfleetAPIConfig{Timeout: "10s", RetryAttempts: 1},
			Steps: []config_loader.Step{
				{Name: "hyperfleetApiBaseUrl", Param: &config_loader.ParamStep{Source: "env.HYPERFLEET_API_BASE_URL"}},
				{Name: "hyperfleetApiVersion", Param: &config_loader.ParamStep{Default: "v1"}},
				{Name: "clusterId", Param: &config_loader.ParamStep{Source: "event.cluster_id"}},
				{
					Name: "clusterStatus",
					APICall: &config_loader.APICallStep{
						Method:  "GET",
						URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterId }}",
						Capture: []config_loader.CaptureField{{Name: "clusterPhase", Field: "status.phase"}},
					},
				},
				{
					Name: "clusterConfigMap",
					When: `clusterPhase == "Ready"`,
					Resource: &config_loader.ResourceStep{
						Manifest: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "cluster-config-{{ .clusterId }}",
								"namespace": testNamespace,
								"labels": map[string]interface{}{
									"hyperfleet.io/cluster-id": "{{ .clusterId }}",
									"hyperfleet.io/managed-by": "{{ .metadata.name }}",
									"app":                      "cluster-config",
								},
								"annotations": map[string]interface{}{
									k8s_client.AnnotationGeneration: "1",
								},
							},
							"data": map[string]interface{}{
								"cluster-id": "{{ .clusterId }}",
							},
						},
						Discovery: &config_loader.DiscoveryConfig{
							Namespace: testNamespace,
							BySelectors: &config_loader.SelectorConfig{
								LabelSelector: map[string]string{
									"hyperfleet.io/cluster-id": "{{ .clusterId }}",
									"app":                      "cluster-config",
								},
							},
						},
					},
				},
			},
		},
	}

	apiClient, err := hyperfleet_api.NewClient(testLog())
	require.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	ctx := context.Background()

	// First execution - should create
	evt := createK8sTestEvent(clusterId)
	result1 := parseAndExecute(t, exec, ctx, evt)
	require.Equal(t, executor.StatusSuccess, result1.Status)
	t.Logf("First execution completed")

	// Verify resource was created
	cmGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	cmName := fmt.Sprintf("cluster-config-%s", clusterId)
	_, err = k8sEnv.Client.GetResource(ctx, cmGVK, testNamespace, cmName)
	require.NoError(t, err, "ConfigMap should exist after first execution")

	// Second execution - should find by labels (but skip due to same generation)
	evt2 := createK8sTestEvent(clusterId)
	result2 := parseAndExecute(t, exec, ctx, evt2)
	require.Equal(t, executor.StatusSuccess, result2.Status)
	t.Logf("Second execution completed (discovered by labels)")
}

// TestExecutor_K8s_RecreateOnChange tests the recreateOnChange behavior
func TestExecutor_K8s_RecreateOnChange(t *testing.T) {
	k8sEnv := SetupK8sTestEnv(t)
	defer k8sEnv.Cleanup(t)

	testNamespace := fmt.Sprintf("executor-recreate-%d", time.Now().Unix())
	k8sEnv.CreateTestNamespace(t, testNamespace)
	defer k8sEnv.CleanupTestNamespace(t, testNamespace)

	mockAPI := newK8sTestAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	clusterId := fmt.Sprintf("recreate-cluster-%d", time.Now().UnixNano())

	// Create config with recreateOnChange
	config := &config_loader.AdapterConfig{
		APIVersion: "hyperfleet.redhat.com/v1alpha1",
		Kind:       "AdapterConfig",
		Metadata: config_loader.Metadata{
			Name:      "recreate-test",
			Namespace: testNamespace,
		},
		Spec: config_loader.AdapterConfigSpec{
			Adapter:       config_loader.AdapterInfo{Version: "1.0.0"},
			HyperfleetAPI: config_loader.HyperfleetAPIConfig{Timeout: "10s", RetryAttempts: 1},
			Steps: []config_loader.Step{
				{Name: "hyperfleetApiBaseUrl", Param: &config_loader.ParamStep{Source: "env.HYPERFLEET_API_BASE_URL"}},
				{Name: "hyperfleetApiVersion", Param: &config_loader.ParamStep{Default: "v1"}},
				{Name: "clusterId", Param: &config_loader.ParamStep{Source: "event.cluster_id"}},
				{
					Name: "clusterStatus",
					APICall: &config_loader.APICallStep{
						Method:  "GET",
						URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterId }}",
						Capture: []config_loader.CaptureField{{Name: "clusterPhase", Field: "status.phase"}},
					},
				},
				{
					Name: "clusterConfigMap",
					When: `clusterPhase == "Ready"`,
					Resource: &config_loader.ResourceStep{
						RecreateOnChange: true, // Enable recreate
						Manifest: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "cluster-config-{{ .clusterId }}",
								"namespace": testNamespace,
								"labels": map[string]interface{}{
									"hyperfleet.io/cluster-id": "{{ .clusterId }}",
								},
								"annotations": map[string]interface{}{
									k8s_client.AnnotationGeneration: "1",
								},
							},
							"data": map[string]interface{}{
								"cluster-id": "{{ .clusterId }}",
							},
						},
						Discovery: &config_loader.DiscoveryConfig{
							Namespace: testNamespace,
							ByName:    "cluster-config-{{ .clusterId }}",
						},
					},
				},
			},
		},
	}

	apiClient, err := hyperfleet_api.NewClient(testLog())
	require.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	ctx := context.Background()

	// First execution - create
	evt := createK8sTestEvent(clusterId)
	result1 := parseAndExecute(t, exec, ctx, evt)
	require.Equal(t, executor.StatusSuccess, result1.Status)

	// Get the original UID
	cmGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	cmName := fmt.Sprintf("cluster-config-%s", clusterId)
	originalCM, err := k8sEnv.Client.GetResource(ctx, cmGVK, testNamespace, cmName)
	require.NoError(t, err)
	originalUID := originalCM.GetUID()
	t.Logf("Original ConfigMap UID: %s", originalUID)

	// Update the config to have generation "2" to trigger recreate
	config.Spec.Steps[4].Resource.Manifest.(map[string]interface{})["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})[k8s_client.AnnotationGeneration] = "2"

	// Recreate executor with updated config
	exec2, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	// Second execution - should recreate (delete + create)
	evt2 := createK8sTestEvent(clusterId)
	result2 := parseAndExecute(t, exec2, ctx, evt2)
	require.Equal(t, executor.StatusSuccess, result2.Status)
	t.Logf("Second execution completed")

	// Verify it's a new resource (different UID)
	recreatedCM, err := k8sEnv.Client.GetResource(ctx, cmGVK, testNamespace, cmName)
	require.NoError(t, err)
	newUID := recreatedCM.GetUID()
	assert.NotEqual(t, originalUID, newUID, "Resource should have new UID after recreate")
	t.Logf("Recreated ConfigMap UID: %s (different from %s)", newUID, originalUID)
}

// TestExecutor_K8s_MultipleResourceTypes tests creating different resource types
func TestExecutor_K8s_MultipleResourceTypes(t *testing.T) {
	k8sEnv := SetupK8sTestEnv(t)
	defer k8sEnv.Cleanup(t)

	testNamespace := fmt.Sprintf("executor-multi-%d", time.Now().Unix())
	k8sEnv.CreateTestNamespace(t, testNamespace)
	defer k8sEnv.CleanupTestNamespace(t, testNamespace)

	mockAPI := newK8sTestAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Execute with default config (ConfigMap + Secret)
	config := createK8sTestConfig(mockAPI.URL(), testNamespace)
	apiClient, err := hyperfleet_api.NewClient(testLog())
	require.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	clusterId := fmt.Sprintf("multi-cluster-%d", time.Now().UnixNano())
	evt := createK8sTestEvent(clusterId)

	result := parseAndExecute(t, exec, context.Background(), evt)

	require.Equal(t, executor.StatusSuccess, result.Status)

	// Verify both resource steps completed
	cmResult := result.GetStepResult("clusterConfigMap")
	require.NotNil(t, cmResult, "clusterConfigMap step should exist")
	assert.False(t, cmResult.Skipped, "ConfigMap step should not be skipped")

	secretResult := result.GetStepResult("clusterSecret")
	require.NotNil(t, secretResult, "clusterSecret step should exist")
	assert.False(t, secretResult.Skipped, "Secret step should not be skipped")

	// Verify we can list resources by labels
	cmGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	selector := fmt.Sprintf("hyperfleet.io/cluster-id=%s", clusterId)
	list, err := k8sEnv.Client.ListResources(context.Background(), cmGVK, testNamespace, selector)
	require.NoError(t, err)
	assert.Len(t, list.Items, 1, "Should find 1 ConfigMap with cluster label")
}

// TestExecutor_K8s_ResourceCreationFailure tests handling of K8s API failures
func TestExecutor_K8s_ResourceCreationFailure(t *testing.T) {
	k8sEnv := SetupK8sTestEnv(t)
	defer k8sEnv.Cleanup(t)

	// Use a namespace that doesn't exist (should fail)
	nonExistentNamespace := "non-existent-namespace-12345"

	mockAPI := newK8sTestAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createK8sTestConfig(mockAPI.URL(), nonExistentNamespace)
	apiClient, err := hyperfleet_api.NewClient(testLog())
	require.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	evt := createK8sTestEvent("failure-test")
	result := parseAndExecute(t, exec, context.Background(), evt)

	// Should fail during resource creation (soft failure mode continues)
	// The overall status depends on whether all steps completed
	t.Logf("Execution status: %s, errors: %v", result.Status, result.Errors)

	// Resource step should have an error
	cmResult := result.GetStepResult("clusterConfigMap")
	require.NotNil(t, cmResult, "clusterConfigMap step should exist")
	assert.NotNil(t, cmResult.Error, "Resource step should have error for non-existent namespace")
	t.Logf("Expected failure: %v", cmResult.Error)

	// Post actions (status report) should still execute due to soft failure model
	statusResponses := mockAPI.GetStatusResponses()
	if len(statusResponses) == 1 {
		status := statusResponses[0]
		t.Logf("K8s error status payload: %+v", status)

		if conditions, ok := status["conditions"].(map[string]interface{}); ok {
			if applied, ok := conditions["applied"].(map[string]interface{}); ok {
				// Status should be false (adapter.executionStatus != "success" when there are errors)
				if applied["status"] == false {
					t.Logf("Correctly reported failure status")
				}
			}
		}
	}
}

// TestExecutor_K8s_WhenClauseSkipsResources tests that when clause skips resources appropriately
func TestExecutor_K8s_WhenClauseSkipsResources(t *testing.T) {
	k8sEnv := SetupK8sTestEnv(t)
	defer k8sEnv.Cleanup(t)

	testNamespace := fmt.Sprintf("executor-when-%d", time.Now().Unix())
	k8sEnv.CreateTestNamespace(t, testNamespace)
	defer k8sEnv.CleanupTestNamespace(t, testNamespace)

	mockAPI := newK8sTestAPIServer(t)
	defer mockAPI.Close()

	// Set cluster to Terminating phase (won't match when clause)
	mockAPI.clusterResponse = map[string]interface{}{
		"metadata": map[string]interface{}{"name": "test-cluster"},
		"spec":     map[string]interface{}{"region": "us-east-1"},
		"status":   map[string]interface{}{"phase": "Terminating"}, // Won't match
	}

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createK8sTestConfig(mockAPI.URL(), testNamespace)
	apiClient, err := hyperfleet_api.NewClient(testLog())
	require.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sEnv.Client).
		WithLogger(k8sEnv.Log).
		Build()
	require.NoError(t, err)

	clusterId := fmt.Sprintf("when-skip-%d", time.Now().UnixNano())
	evt := createK8sTestEvent(clusterId)

	result := parseAndExecute(t, exec, context.Background(), evt)

	// Should be success (when clause skip is valid outcome in soft failure model)
	assert.Equal(t, executor.StatusSuccess, result.Status, "Should be success when resources skipped via when clause")

	// Resource steps should be skipped
	cmResult := result.GetStepResult("clusterConfigMap")
	require.NotNil(t, cmResult, "clusterConfigMap step should exist")
	assert.True(t, cmResult.Skipped, "ConfigMap step should be skipped when phase is Terminating")
	t.Logf("ConfigMap step skipped: %v", cmResult.Skipped)

	secretResult := result.GetStepResult("clusterSecret")
	require.NotNil(t, secretResult, "clusterSecret step should exist")
	assert.True(t, secretResult.Skipped, "Secret step should be skipped when phase is Terminating")

	// Status report step should still execute
	statusResponses := mockAPI.GetStatusResponses()
	require.Len(t, statusResponses, 1, "Should have reported status even when resources skipped")
	t.Logf("Status reported after resources skipped: %+v", statusResponses[0])
}
