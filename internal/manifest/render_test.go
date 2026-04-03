package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderManifestData(t *testing.T) {
	tests := []struct {
		data        map[string]interface{}
		params      map[string]interface{}
		expected    map[string]interface{}
		name        string
		expectError bool
	}{
		{
			name: "simple value rendering",
			data: map[string]interface{}{
				"name": "{{ .clusterId }}",
				"kind": "ConfigMap",
			},
			params: map[string]interface{}{"clusterId": "cluster-123"},
			expected: map[string]interface{}{
				"name": "cluster-123",
				"kind": "ConfigMap",
			},
		},
		{
			name: "nested map rendering",
			data: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "{{ .name }}",
					"namespace": "{{ .namespace }}",
				},
			},
			params: map[string]interface{}{"name": "test", "namespace": "default"},
			expected: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "test",
					"namespace": "default",
				},
			},
		},
		{
			name: "key rendering",
			data: map[string]interface{}{
				"{{ .labelKey }}": "value",
			},
			params: map[string]interface{}{"labelKey": "app"},
			expected: map[string]interface{}{
				"app": "value",
			},
		},
		{
			name: "no template expressions passes through",
			data: map[string]interface{}{
				"static": "value",
			},
			params: map[string]interface{}{},
			expected: map[string]interface{}{
				"static": "value",
			},
		},
		{
			name: "slice rendering",
			data: map[string]interface{}{
				"items": []interface{}{"{{ .a }}", "{{ .b }}"},
			},
			params: map[string]interface{}{"a": "x", "b": "y"},
			expected: map[string]interface{}{
				"items": []interface{}{"x", "y"},
			},
		},
		{
			name: "non-string values pass through",
			data: map[string]interface{}{
				"count":   42,
				"enabled": true,
			},
			params: map[string]interface{}{},
			expected: map[string]interface{}{
				"count":   42,
				"enabled": true,
			},
		},
		{
			name: "map[interface{}]interface{} values are converted and rendered",
			data: map[string]interface{}{
				"metadata": map[interface{}]interface{}{
					"name":      "{{ .name }}",
					"namespace": "default",
				},
			},
			params: map[string]interface{}{"name": "test"},
			expected: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "test",
					"namespace": "default",
				},
			},
		},
		{
			name: "missing variable returns error",
			data: map[string]interface{}{
				"name": "{{ .missing }}",
			},
			params:      map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderManifestData(tt.data, tt.params)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
