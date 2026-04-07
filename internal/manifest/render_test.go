package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToYAMLString(t *testing.T) {
	tests := []struct {
		name        string
		manifest    interface{}
		wantContain string
		wantErr     bool
	}{
		{
			name:        "string passthrough",
			manifest:    "apiVersion: v1\nkind: ConfigMap",
			wantContain: "apiVersion: v1",
		},
		{
			name: "map to YAML",
			manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
			wantContain: "apiVersion: v1",
		},
		{
			name: "map with interface keys",
			manifest: map[interface{}]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
			wantContain: "apiVersion: v1",
		},
		{
			name:     "unsupported type",
			manifest: 42,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToYAMLString(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, result, tt.wantContain)
		})
	}
}

func TestRenderStringManifest(t *testing.T) {
	tests := []struct {
		name        string
		manifest    string
		params      map[string]interface{}
		wantContain string
		wantErr     bool
	}{
		{
			name:        "simple template rendering",
			manifest:    "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: \"{{ .name }}\"",
			params:      map[string]interface{}{"name": "test"},
			wantContain: `"name":"test"`,
		},
		{
			name:     "empty manifest",
			manifest: "",
			params:   map[string]interface{}{},
			wantErr:  true,
		},
		{
			name:     "missing variable",
			manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: \"{{ .missing }}\"",
			params:   map[string]interface{}{},
			wantErr:  true,
		},
		{
			name:        "no template expressions",
			manifest:    "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: static",
			params:      map[string]interface{}{},
			wantContain: `"name":"static"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderStringManifest(tt.manifest, tt.params)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, string(result), tt.wantContain)
		})
	}
}
