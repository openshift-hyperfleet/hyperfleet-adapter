package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		data        map[string]interface{}
		expected    string
		expectError bool
	}{
		{
			name:     "simple variable",
			template: "Hello {{ .name }}!",
			data:     map[string]interface{}{"name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "no template markers returns as-is",
			template: "plain text",
			data:     map[string]interface{}{},
			expected: "plain text",
		},
		{
			name:     "nested variable",
			template: "{{ .cluster.id }}",
			data: map[string]interface{}{
				"cluster": map[string]interface{}{"id": "test-123"},
			},
			expected: "test-123",
		},
		{
			name:        "missing variable returns error",
			template:    "{{ .missing }}",
			data:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.data)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderTemplateBytes(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		data        map[string]interface{}
		expected    []byte
		expectError bool
	}{
		{
			name:     "simple template",
			template: "Hello {{ .name }}!",
			data:     map[string]interface{}{"name": "World"},
			expected: []byte("Hello World!"),
		},
		{
			name:     "no template markers",
			template: "plain text",
			data:     map[string]interface{}{},
			expected: []byte("plain text"),
		},
		{
			name:     "JSON body template",
			template: `{"id": "{{ .clusterId }}", "region": "{{ .region }}"}`,
			data:     map[string]interface{}{"clusterId": "cluster-123", "region": "us-east-1"},
			expected: []byte(`{"id": "cluster-123", "region": "us-east-1"}`),
		},
		{
			name:        "missing variable returns error",
			template:    "{{ .missing }}",
			data:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplateBytes(tt.template, tt.data)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
