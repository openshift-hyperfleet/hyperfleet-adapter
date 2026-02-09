package executor

import (
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepCopyMap_BasicTypes(t *testing.T) {
	original := map[string]interface{}{
		"string": "hello",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
	}

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

	// Verify values are copied correctly
	assert.Equal(t, "hello", copied["string"])
	assert.Equal(t, 42, copied["int"]) // copystructure preserves int (unlike JSON which converts to float64)
	assert.Equal(t, 3.14, copied["float"])
	assert.Equal(t, true, copied["bool"])
	assert.Nil(t, copied["null"])

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

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

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

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

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

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

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

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

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

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

	// Modify the copied nested map
	copiedNested := copied["nested"].(map[string]interface{})
	copiedNested["mutable"] = "MUTATED"

	// Original should NOT be affected (deep copy works with copystructure)
	assert.Equal(t, "original", nested["mutable"],
		"Deep copy: original nested map should NOT be affected by mutation")
}

func TestDeepCopyMap_EmptyMap(t *testing.T) {
	original := map[string]interface{}{}
	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

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

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

	assert.Equal(t, "value", copied["string"])

	// Verify deep copy works
	copiedNested := copied["nested"].(map[string]interface{})
	copiedNested["key"] = "modified"

	originalNested := original["nested"].(map[string]interface{})
	assert.Equal(t, "nested_value", originalNested["key"], "Original should not be modified")
}

func TestDeepCopyMap_NilMap(t *testing.T) {
	copied, err := utils.DeepCopyMap(nil)
	require.NoError(t, err)
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

	copied, err := utils.DeepCopyMap(original)
	require.NoError(t, err)

	// Modify copied manifest
	copiedMetadata := copied["metadata"].(map[string]interface{})
	copiedLabels := copiedMetadata["labels"].(map[string]interface{})
	copiedLabels["app"] = "modified"

	// Verify original is NOT modified
	originalMetadata := original["metadata"].(map[string]interface{})
	originalLabels := originalMetadata["labels"].(map[string]interface{})
	assert.Equal(t, "test", originalLabels["app"], "Original manifest should not be modified")
}

// TestDeepCopyMap_RealWorldContext ensures the function is used correctly in context
func TestDeepCopyMap_RealWorldContext(t *testing.T) {
	// This simulates how deepCopyMap is used in executeResource
	manifestData := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": "{{ .namespace }}",
		},
	}

	// Deep copy before template rendering
	copied, err := utils.DeepCopyMap(manifestData)
	require.NoError(t, err)

	// Simulate template rendering modifying the copy
	copiedMetadata := copied["metadata"].(map[string]interface{})
	copiedMetadata["name"] = "rendered-namespace"

	// Original template should remain unchanged for next iteration
	originalMetadata := manifestData["metadata"].(map[string]interface{})
	assert.Equal(t, "{{ .namespace }}", originalMetadata["name"])
}

// TestDeepCopyMapWithFallback tests the fallback version
func TestDeepCopyMapWithFallback(t *testing.T) {
	original := map[string]interface{}{
		"key": "value",
		"nested": map[string]interface{}{
			"inner": "deep",
		},
	}

	copied := utils.DeepCopyMapWithFallback(original)
	assert.NotNil(t, copied)
	assert.Equal(t, "value", copied["key"])

	// Verify it's a deep copy
	copiedNested := copied["nested"].(map[string]interface{})
	copiedNested["inner"] = "modified"

	originalNested := original["nested"].(map[string]interface{})
	assert.Equal(t, "deep", originalNested["inner"], "Original should not be modified")
}

func TestDeepCopyMapWithFallback_NilMap(t *testing.T) {
	copied := utils.DeepCopyMapWithFallback(nil)
	assert.Nil(t, copied)
}
