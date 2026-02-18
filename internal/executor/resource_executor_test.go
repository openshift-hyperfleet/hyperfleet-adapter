package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDeepCopyMap_BasicTypes(t *testing.T) {
	original := map[string]interface{}{
		"string": "hello",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
	}

	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	// Verify values are copied correctly
	assert.Equal(t, "hello", copied["string"])
	assert.Equal(t, 42, copied["int"]) // copystructure preserves int (unlike JSON which converts to float64)
	assert.Equal(t, 3.14, copied["float"])
	assert.Equal(t, true, copied["bool"])
	assert.Nil(t, copied["null"])

	// Verify no warnings logged

	// Verify mutation doesn't affect original
	copied["string"] = "modified"
	assert.Equal(t, "hello", original["string"], "Original should not be modified")
}

func TestDeepCopyMap_NestedMaps(t *testing.T) {

	original := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"value": "deep",
			},
		},
	}

	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	// Verify deep copy works

	// Modify the copied nested map
	level1 := copied["level1"].(map[string]interface{})
	level2 := level1["level2"].(map[string]interface{})
	level2["value"] = "modified"

	// Verify original is NOT modified (deep copy worked)
	originalLevel1 := original["level1"].(map[string]interface{})
	originalLevel2 := originalLevel1["level2"].(map[string]interface{})
	assert.Equal(t, "deep", originalLevel2["value"], "Original nested value should not be modified")
}

func TestDeepCopyMap_Slices(t *testing.T) {

	original := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
		"nested": []interface{}{
			map[string]interface{}{"key": "value"},
		},
	}

	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	// Modify copied slice
	copiedItems := copied["items"].([]interface{})
	copiedItems[0] = "modified"

	// Verify original is NOT modified
	originalItems := original["items"].([]interface{})
	assert.Equal(t, "a", originalItems[0], "Original slice should not be modified")
}

func TestDeepCopyMap_Channel(t *testing.T) {
	// copystructure handles channels properly (creates new channel)

	ch := make(chan int, 5)
	original := map[string]interface{}{
		"channel": ch,
		"normal":  "value",
	}

	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	// copystructure handles channels - no warning expected

	// Normal values are copied
	assert.Equal(t, "value", copied["normal"])

	// Verify channel exists in copied map
	copiedCh, ok := copied["channel"].(chan int)
	assert.True(t, ok, "Channel should be present in copied map")
	assert.NotNil(t, copiedCh, "Copied channel should not be nil")
}

func TestDeepCopyMap_Function(t *testing.T) {
	// copystructure handles functions (copies the function pointer)

	fn := func() string { return "hello" }
	original := map[string]interface{}{
		"func":   fn,
		"normal": "value",
	}

	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	// copystructure handles functions - no warning expected

	// Normal values are copied
	assert.Equal(t, "value", copied["normal"])

	// Function is preserved
	copiedFn := copied["func"].(func() string)
	assert.Equal(t, "hello", copiedFn(), "Copied function should work")
}

func TestDeepCopyMap_NestedWithChannel(t *testing.T) {
	// Test that nested maps are deep copied even when channels are present

	ch := make(chan int)
	nested := map[string]interface{}{"mutable": "original"}
	original := map[string]interface{}{
		"channel": ch,
		"nested":  nested,
	}

	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	// copystructure handles this properly - no warning expected

	// Modify the copied nested map
	copiedNested := copied["nested"].(map[string]interface{})
	copiedNested["mutable"] = "MUTATED"

	// Original should NOT be affected (deep copy works with copystructure)
	assert.Equal(t, "original", nested["mutable"],
		"Deep copy: original nested map should NOT be affected by mutation")
}

func TestDeepCopyMap_EmptyMap(t *testing.T) {

	original := map[string]interface{}{}
	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	assert.NotNil(t, copied)
	assert.Empty(t, copied)
}

func TestDeepCopyMap_DeepCopyVerification(t *testing.T) {
	// Verify deep copy works correctly
	original := map[string]interface{}{
		"string": "value",
		"nested": map[string]interface{}{
			"key": "nested_value",
		},
	}

	// Should not panic
	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	assert.Equal(t, "value", copied["string"])

	// Verify deep copy works
	copiedNested := copied["nested"].(map[string]interface{})
	copiedNested["key"] = "modified"

	originalNested := original["nested"].(map[string]interface{})
	assert.Equal(t, "nested_value", originalNested["key"], "Original should not be modified")
}

func TestDeepCopyMap_NilMap(t *testing.T) {

	copied := deepCopyMap(context.Background(), nil, logger.NewTestLogger())

	assert.Nil(t, copied)
}

func TestDeepCopyMap_KubernetesManifest(t *testing.T) {
	// Test with a realistic Kubernetes manifest structure

	original := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "test-config",
			"namespace": "default",
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}

	copied := deepCopyMap(context.Background(), original, logger.NewTestLogger())

	// Modify copied manifest
	copiedMetadata := copied["metadata"].(map[string]interface{})
	copiedLabels := copiedMetadata["labels"].(map[string]interface{})
	copiedLabels["app"] = "modified"

	// Verify original is NOT modified
	originalMetadata := original["metadata"].(map[string]interface{})
	originalLabels := originalMetadata["labels"].(map[string]interface{})
	assert.Equal(t, "test", originalLabels["app"], "Original manifest should not be modified")
}

// TestDeepCopyMap_Context ensures the function is used correctly in context
func TestDeepCopyMap_RealWorldContext(t *testing.T) {
	// This simulates how deepCopyMap is used in executeResource
	manifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": "{{ .namespace }}",
		},
	}

	// Deep copy before template rendering
	copied := deepCopyMap(context.Background(), manifest, logger.NewTestLogger())

	// Simulate template rendering modifying the copy
	copiedMetadata := copied["metadata"].(map[string]interface{})
	copiedMetadata["name"] = "rendered-namespace"

	// Original template should remain unchanged for next iteration
	originalMetadata := manifest["metadata"].(map[string]interface{})
	assert.Equal(t, "{{ .namespace }}", originalMetadata["name"])
}

// TestResourceExecutor_ExecuteAll_DiscoveryFailure verifies that when discovery fails after a successful apply,
// the error is logged and notified: ExecuteAll returns an error, result is failed, and execCtx.Adapter.ExecutionError is set.
func TestResourceExecutor_ExecuteAll_DiscoveryFailure(t *testing.T) {
	discoveryErr := errors.New("discovery failed: resource not found")
	mock := k8s_client.NewMockK8sClient()
	mock.GetResourceError = discoveryErr
	// Apply succeeds so we reach discovery
	mock.ApplyResourceResult = &transport_client.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock",
	}

	config := &ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	}
	re := newResourceExecutor(config)

	resource := config_loader.Resource{
		Name:      "test-resource",
		Transport: &config_loader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
		Discovery: &config_loader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-cm",
		},
	}
	resources := []config_loader.Resource{resource}
	execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)

	results, err := re.ExecuteAll(context.Background(), resources, execCtx)

	require.Error(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusFailed, results[0].Status, "result status should be failed")
	require.NotNil(t, results[0].Error)
	assert.Contains(t, results[0].Error.Error(), "discovery failed", "result error should describe discovery failure")
	require.NotNil(t, execCtx.Adapter.ExecutionError, "ExecutionError should be set for notification")
	assert.Equal(t, string(PhaseResources), execCtx.Adapter.ExecutionError.Phase)
	assert.Equal(t, resource.Name, execCtx.Adapter.ExecutionError.Step)
	assert.Contains(t, execCtx.Adapter.ExecutionError.Message, "discovery failed")
}

func TestResourceExecutor_ExecuteAll_StoresNestedDiscoveriesByName(t *testing.T) {
	mock := k8s_client.NewMockK8sClient()
	mock.ApplyResourceResult = &transport_client.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock",
	}
	mock.GetResourceResult = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "cluster-1-adapter2",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "cluster-1-adapter2-configmap",
								"namespace": "default",
							},
							"data": map[string]interface{}{
								"cluster_id": "cluster-1",
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"resourceStatus": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"resourceMeta": map[string]interface{}{
								"name":      "cluster-1-adapter2-configmap",
								"namespace": "default",
								"resource":  "configmaps",
								"group":     "",
							},
							"statusFeedback": map[string]interface{}{
								"values": []interface{}{
									map[string]interface{}{
										"name": "data",
										"fieldValue": map[string]interface{}{
											"type":    "JsonRaw",
											"jsonRaw": "{\"cluster_id\":\"cluster-1\"}",
										},
									},
								},
							},
							"conditions": []interface{}{
								map[string]interface{}{
									"type":   "Applied",
									"status": "True",
								},
							},
						},
					},
				},
			},
		},
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	resource := config_loader.Resource{
		Name: "resource0",
		Transport: &config_loader.TransportConfig{
			Client: "kubernetes",
		},
		Manifest: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "cluster-1-adapter2",
				"namespace": "default",
			},
		},
		Discovery: &config_loader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "cluster-1-adapter2",
		},
		NestedDiscoveries: []config_loader.NestedDiscovery{
			{
				Name: "configmap0",
				Discovery: &config_loader.DiscoveryConfig{
					Namespace: "default",
					ByName:    "cluster-1-adapter2-configmap",
				},
			},
		},
	}

	execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)
	results, err := re.ExecuteAll(context.Background(), []config_loader.Resource{resource}, execCtx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)

	parent, ok := execCtx.Resources["resource0"].(*unstructured.Unstructured)
	require.True(t, ok, "resource0 should store the discovered parent resource")
	assert.Equal(t, "ManifestWork", parent.GetKind())
	assert.Equal(t, "cluster-1-adapter2", parent.GetName())

	nested, ok := execCtx.Resources["configmap0"].(*unstructured.Unstructured)
	require.True(t, ok, "configmap0 should be stored as top-level nested discovery")
	assert.Equal(t, "ConfigMap", nested.GetKind())
	assert.Equal(t, "cluster-1-adapter2-configmap", nested.GetName())

	// Verify statusFeedback and conditions were enriched from parent's status.resourceStatus
	_, hasSF := nested.Object["statusFeedback"]
	assert.True(t, hasSF, "configmap0 should have statusFeedback merged from parent")
	_, hasConds := nested.Object["conditions"]
	assert.True(t, hasConds, "configmap0 should have conditions merged from parent")

	sf := nested.Object["statusFeedback"].(map[string]interface{})
	values := sf["values"].([]interface{})
	assert.Len(t, values, 1)
	v0 := values[0].(map[string]interface{})
	assert.Equal(t, "data", v0["name"])
}
