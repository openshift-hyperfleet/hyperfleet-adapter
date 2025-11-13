package k8sclient

import (
	"strings"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		variables map[string]interface{}
		want      string
		wantErr   bool
	}{
		{
			name:     "simple variable substitution",
			template: "Hello {{ .name }}",
			variables: map[string]interface{}{
				"name": "World",
			},
			want:    "Hello World",
			wantErr: false,
		},
		{
			name:     "multiple variables",
			template: "cluster-{{ .clusterId }}-{{ .region }}",
			variables: map[string]interface{}{
				"clusterId": "abc123",
				"region":    "us-east-1",
			},
			want:    "cluster-abc123-us-east-1",
			wantErr: false,
		},
		{
			name:     "nested object access",
			template: "{{ .cluster.metadata.name }}",
			variables: map[string]interface{}{
				"cluster": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test-cluster",
					},
				},
			},
			want:    "test-cluster",
			wantErr: false,
		},
		{
			name:     "sprig functions - upper",
			template: "{{ .name | upper }}",
			variables: map[string]interface{}{
				"name": "hello",
			},
			want:    "HELLO",
			wantErr: false,
		},
		{
			name:     "sprig functions - default",
			template: "{{ .missing | default \"fallback\" }}",
			variables: map[string]interface{}{
				"other": "value",
			},
			want:    "fallback",
			wantErr: false,
		},
		{
			name:     "conditional logic",
			template: "{{ if eq .env \"prod\" }}production{{ else }}development{{ end }}",
			variables: map[string]interface{}{
				"env": "prod",
			},
			want:    "production",
			wantErr: false,
		},
		{
			name:     "invalid template syntax",
			template: "{{ .name ",
			variables: map[string]interface{}{
				"name": "test",
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderTemplate(tt.template, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RenderTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseYAMLToUnstructured(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		checks  func(*testing.T, map[string]interface{})
	}{
		{
			name: "valid namespace",
			yaml: `
apiVersion: v1
kind: Namespace
metadata:
  name: test-namespace
  labels:
    app: test
`,
			wantErr: false,
			checks: func(t *testing.T, obj map[string]interface{}) {
				if obj["kind"] != "Namespace" {
					t.Errorf("expected kind Namespace, got %v", obj["kind"])
				}
				metadata := obj["metadata"].(map[string]interface{})
				if metadata["name"] != "test-namespace" {
					t.Errorf("expected name test-namespace, got %v", metadata["name"])
				}
			},
		},
		{
			name: "valid deployment",
			yaml: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: default
spec:
  replicas: 3
`,
			wantErr: false,
			checks: func(t *testing.T, obj map[string]interface{}) {
				if obj["kind"] != "Deployment" {
					t.Errorf("expected kind Deployment, got %v", obj["kind"])
				}
				spec := obj["spec"].(map[string]interface{})
				replicas := spec["replicas"].(int64)
				if replicas != 3 {
					t.Errorf("expected replicas 3, got %v", replicas)
				}
			},
		},
		{
			name:    "invalid yaml",
			yaml:    "not: valid: yaml:",
			wantErr: true,
		},
		{
			name:    "empty yaml",
			yaml:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseYAMLToUnstructured(tt.yaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseYAMLToUnstructured() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checks != nil {
				tt.checks(t, got.Object)
			}
		})
	}
}

func TestRenderAndParseResource(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		variables map[string]interface{}
		wantErr   bool
		checks    func(*testing.T, map[string]interface{})
	}{
		{
			name: "render and parse namespace",
			template: `
apiVersion: v1
kind: Namespace
metadata:
  name: cluster-{{ .clusterId }}
  labels:
    cluster-id: "{{ .clusterId }}"
`,
			variables: map[string]interface{}{
				"clusterId": "abc123",
			},
			wantErr: false,
			checks: func(t *testing.T, obj map[string]interface{}) {
				metadata := obj["metadata"].(map[string]interface{})
				if metadata["name"] != "cluster-abc123" {
					t.Errorf("expected name cluster-abc123, got %v", metadata["name"])
				}
				labels := metadata["labels"].(map[string]interface{})
				if labels["cluster-id"] != "abc123" {
					t.Errorf("expected cluster-id abc123, got %v", labels["cluster-id"])
				}
			},
		},
		{
			name: "render with sprig functions",
			template: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .name | lower }}
  namespace: {{ .namespace | default "default" }}
data:
  key: {{ .value | quote }}
`,
			variables: map[string]interface{}{
				"name":  "TEST-CONFIG",
				"value": "test-value",
			},
			wantErr: false,
			checks: func(t *testing.T, obj map[string]interface{}) {
				metadata := obj["metadata"].(map[string]interface{})
				if metadata["name"] != "test-config" {
					t.Errorf("expected name test-config, got %v", metadata["name"])
				}
				if metadata["namespace"] != "default" {
					t.Errorf("expected namespace default, got %v", metadata["namespace"])
				}
			},
		},
		{
			name:     "invalid template",
			template: "{{ .missing.field }}",
			variables: map[string]interface{}{
				"other": "value",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderAndParseResource(tt.template, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderAndParseResource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checks != nil {
				tt.checks(t, got.Object)
			}
		})
	}
}

func TestRenderDiscoverySelectors(t *testing.T) {
	tests := []struct {
		name      string
		selectors map[string]string
		variables map[string]interface{}
		want      map[string]string
		wantErr   bool
	}{
		{
			name: "simple selector rendering",
			selectors: map[string]string{
				"app":        "myapp",
				"cluster-id": "{{ .clusterId }}",
			},
			variables: map[string]interface{}{
				"clusterId": "abc123",
			},
			want: map[string]string{
				"app":        "myapp",
				"cluster-id": "abc123",
			},
			wantErr: false,
		},
		{
			name: "multiple variable selectors",
			selectors: map[string]string{
				"hyperfleet.io/cluster-id": "{{ .clusterId }}",
				"hyperfleet.io/region":     "{{ .region }}",
				"hyperfleet.io/env":        "{{ .env }}",
			},
			variables: map[string]interface{}{
				"clusterId": "abc123",
				"region":    "us-east-1",
				"env":       "production",
			},
			want: map[string]string{
				"hyperfleet.io/cluster-id": "abc123",
				"hyperfleet.io/region":     "us-east-1",
				"hyperfleet.io/env":        "production",
			},
			wantErr: false,
		},
		{
			name: "selector key with template",
			selectors: map[string]string{
				"{{ .labelPrefix }}/cluster-id": "{{ .clusterId }}",
			},
			variables: map[string]interface{}{
				"labelPrefix": "hyperfleet.io",
				"clusterId":   "abc123",
			},
			want: map[string]string{
				"hyperfleet.io/cluster-id": "abc123",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderDiscoverySelectors(tt.selectors, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderDiscoverySelectors() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				for key, want := range tt.want {
					if got[key] != want {
						t.Errorf("RenderDiscoverySelectors()[%s] = %v, want %v", key, got[key], want)
					}
				}
			}
		})
	}
}

func TestBuildLabelSelector(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name: "single label",
			labels: map[string]string{
				"app": "myapp",
			},
			want: "app=myapp",
		},
		{
			name: "multiple labels",
			labels: map[string]string{
				"app":        "myapp",
				"env":        "production",
				"cluster-id": "abc123",
			},
			// Note: map iteration order is non-deterministic, so we check contains
		},
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLabelSelector(tt.labels)
			
			if len(tt.labels) == 0 {
				if got != tt.want {
					t.Errorf("BuildLabelSelector() = %v, want %v", got, tt.want)
				}
				return
			}
			
			if len(tt.labels) == 1 {
				if got != tt.want {
					t.Errorf("BuildLabelSelector() = %v, want %v", got, tt.want)
				}
				return
			}
			
			// For multiple labels, just verify all are present
			for key, value := range tt.labels {
				expected := key + "=" + value
				if !strings.Contains(got, expected) {
					t.Errorf("BuildLabelSelector() = %v, should contain %v", got, expected)
				}
			}
		})
	}
}

