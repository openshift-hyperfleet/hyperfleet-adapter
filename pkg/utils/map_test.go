package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testModifiedValue = "modified"

func TestConvertToStringKeyMap(t *testing.T) {
	tests := []struct {
		expected map[string]interface{}
		input    map[interface{}]interface{}
		name     string
	}{
		{
			name:     "empty map",
			input:    map[interface{}]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "simple string keys",
			input: map[interface{}]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "integer keys",
			input: map[interface{}]interface{}{
				1: "one",
				2: "two",
			},
			expected: map[string]interface{}{
				"1": "one",
				"2": "two",
			},
		},
		{
			name: "nested map",
			input: map[interface{}]interface{}{
				"outer": map[interface{}]interface{}{
					"inner": "value",
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name: "nested slice",
			input: map[interface{}]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
			expected: map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
		},
		{
			name: "deeply nested structure",
			input: map[interface{}]interface{}{
				"level1": map[interface{}]interface{}{
					"level2": map[interface{}]interface{}{
						"level3": "deep value",
					},
				},
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "deep value",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToStringKeyMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertSlice(t *testing.T) {
	tests := []struct {
		name     string
		expected []interface{}
		input    []interface{}
	}{
		{
			name:     "empty slice",
			input:    []interface{}{},
			expected: []interface{}{},
		},
		{
			name:     "simple values",
			input:    []interface{}{"a", "b", "c"},
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name: "nested maps in slice",
			input: []interface{}{
				map[interface{}]interface{}{"key": "value1"},
				map[interface{}]interface{}{"key": "value2"},
			},
			expected: []interface{}{
				map[string]interface{}{"key": "value1"},
				map[string]interface{}{"key": "value2"},
			},
		},
		{
			name: "mixed types",
			input: []interface{}{
				"string",
				123,
				map[interface{}]interface{}{"nested": "map"},
				[]interface{}{"nested", "slice"},
			},
			expected: []interface{}{
				"string",
				123,
				map[string]interface{}{"nested": "map"},
				[]interface{}{"nested", "slice"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeepCopyMap_BasicTypes(t *testing.T) {
	original := map[string]interface{}{
		"string": "hello",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
	}

	copied, err := DeepCopyMap(original)
	require.NoError(t, err)

	assert.Equal(t, "hello", copied["string"])
	assert.Equal(t, 42, copied["int"])
	assert.Equal(t, 3.14, copied["float"])
	assert.Equal(t, true, copied["bool"])
	assert.Nil(t, copied["null"])

	// Mutation doesn't affect original
	copied["string"] = testModifiedValue
	assert.Equal(t, "hello", original["string"])
}

func TestDeepCopyMap_NestedMaps(t *testing.T) {
	original := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"value": "deep",
			},
		},
	}

	copied, err := DeepCopyMap(original)
	require.NoError(t, err)

	level1 := copied["level1"].(map[string]interface{})
	level2 := level1["level2"].(map[string]interface{})
	level2["value"] = testModifiedValue

	originalLevel2 := original["level1"].(map[string]interface{})["level2"].(map[string]interface{})
	assert.Equal(t, "deep", originalLevel2["value"])
}

func TestDeepCopyMap_Slices(t *testing.T) {
	original := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
		"nested": []interface{}{
			map[string]interface{}{"key": "value"},
		},
	}

	copied, err := DeepCopyMap(original)
	require.NoError(t, err)

	copiedItems := copied["items"].([]interface{})
	copiedItems[0] = testModifiedValue

	originalItems := original["items"].([]interface{})
	assert.Equal(t, "a", originalItems[0])
}

func TestDeepCopyMap_NilMap(t *testing.T) {
	copied, err := DeepCopyMap(nil)
	require.NoError(t, err)
	assert.Nil(t, copied)
}

func TestDeepCopyMap_EmptyMap(t *testing.T) {
	original := map[string]interface{}{}
	copied, err := DeepCopyMap(original)
	require.NoError(t, err)
	assert.NotNil(t, copied)
	assert.Empty(t, copied)
}

func TestDeepCopyMap_KubernetesManifest(t *testing.T) {
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
		},
	}

	copied, err := DeepCopyMap(original)
	require.NoError(t, err)

	copiedLabels := copied["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
	copiedLabels["app"] = testModifiedValue

	originalLabels := original["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
	assert.Equal(t, "test", originalLabels["app"])
}

func TestDeepCopyMapWithFallback(t *testing.T) {
	original := map[string]interface{}{
		"key": "value",
		"nested": map[string]interface{}{
			"inner": "data",
		},
	}

	copied := DeepCopyMapWithFallback(original)
	assert.Equal(t, "value", copied["key"])

	// Verify deep copy
	copiedNested := copied["nested"].(map[string]interface{})
	copiedNested["inner"] = testModifiedValue

	originalNested := original["nested"].(map[string]interface{})
	assert.Equal(t, "data", originalNested["inner"])
}

func TestDeepCopyMapWithFallback_Nil(t *testing.T) {
	copied := DeepCopyMapWithFallback(nil)
	assert.Nil(t, copied)
}
