package config_loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseConfig returns a minimal valid AdapterConfig for testing.
// Tests can modify the returned config to set up specific scenarios.
func baseConfig() *AdapterConfig {
	return &AdapterConfig{
		APIVersion: "hyperfleet.redhat.com/v1alpha1",
		Kind:       "AdapterConfig",
		Metadata:   Metadata{Name: "test-adapter"},
		Spec: AdapterConfigSpec{
			Adapter:       AdapterInfo{Version: "1.0.0"},
			HyperfleetAPI: HyperfleetAPIConfig{BaseURL: "https://test.example.com", Timeout: "5s"},
			Kubernetes:    KubernetesConfig{APIVersion: "v1"},
		},
	}
}

func TestValidateTemplateVariables(t *testing.T) {
	t.Run("defined variables", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "clusterId", Param: &ParamStep{Source: "event.cluster_id"}},
			{Name: "apiUrl", Param: &ParamStep{Source: "env.API_URL"}},
			{
				Name:    "checkCluster",
				APICall: &APICallStep{Method: "GET", URL: "{{ .apiUrl }}/clusters/{{ .clusterId }}"},
			},
		}
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("undefined variable in URL", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "clusterId", Param: &ParamStep{Source: "event.cluster_id"}},
			{
				Name:    "checkCluster",
				APICall: &APICallStep{Method: "GET", URL: "{{ .undefinedVar }}/clusters/{{ .clusterId }}"},
			},
		}
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefined template variable \"undefinedVar\"")
	})

	t.Run("undefined variable in resource manifest", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "clusterId", Param: &ParamStep{Source: "event.cluster_id"}},
			{
				Name: "testNs",
				Resource: &ResourceStep{
					Manifest: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"metadata":   map[string]interface{}{"name": "ns-{{ .undefinedVar }}"},
					},
					Discovery: &DiscoveryConfig{Namespace: "*", ByName: "ns-{{ .clusterId }}"},
				},
			},
		}
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefined template variable \"undefinedVar\"")
	})

	t.Run("captured variable is available for resources", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "apiUrl", Param: &ParamStep{Source: "env.API_URL"}},
			{
				Name:    "getCluster",
				APICall: &APICallStep{Method: "GET", URL: "{{ .apiUrl }}/clusters", Capture: []CaptureField{{Name: "clusterName", Field: "metadata.name"}}},
			},
			{
				Name: "testNs",
				Resource: &ResourceStep{
					Manifest: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"metadata":   map[string]interface{}{"name": "ns-{{ .clusterName }}"},
					},
					Discovery: &DiscoveryConfig{Namespace: "*", ByName: "ns-{{ .clusterName }}"},
				},
			},
		}
		assert.NoError(t, newValidator(cfg).Validate())
	})
}

func TestValidateCELExpressions(t *testing.T) {
	// Helper to create config with a CEL expression in when clause
	withWhen := func(expr string) *AdapterConfig {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "clusterPhase", Param: &ParamStep{Value: "Ready"}},
			{Name: "check", When: expr, Log: &LogStep{Message: "condition met"}},
		}
		return cfg
	}

	t.Run("valid CEL expression in when clause", func(t *testing.T) {
		cfg := withWhen(`clusterPhase == "Ready" || clusterPhase == "Provisioning"`)
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("invalid CEL expression - syntax error", func(t *testing.T) {
		cfg := withWhen(`clusterPhase ==== "Ready"`)
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})

	t.Run("valid CEL with has() function", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "cluster", Param: &ParamStep{Value: map[string]interface{}{"status": map[string]interface{}{"phase": "Ready"}}}},
			{Name: "check", When: `has(cluster.status) && cluster.status.phase == "Ready"`, Log: &LogStep{Message: "ok"}},
		}
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("valid CEL in param expression", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "base", Param: &ParamStep{Value: "http://example.com"}},
			{Name: "fullUrl", Param: &ParamStep{Expression: `base + "/api/v1"`}},
		}
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("invalid CEL in param expression", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "computed", Param: &ParamStep{Expression: `invalid )) syntax`}},
		}
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})
}

func TestValidateK8sManifests(t *testing.T) {
	// Helper to create config with a resource manifest
	withResource := func(manifest map[string]interface{}) *AdapterConfig {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{{
			Name: "testResource",
			Resource: &ResourceStep{
				Manifest:  manifest,
				Discovery: &DiscoveryConfig{Namespace: "*", ByName: "test"},
			},
		}}
		return cfg
	}

	// Complete valid manifest
	validManifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]interface{}{"name": "test-namespace", "labels": map[string]interface{}{"app": "test"}},
	}

	t.Run("valid K8s manifest", func(t *testing.T) {
		cfg := withResource(validManifest)
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("missing apiVersion in manifest", func(t *testing.T) {
		cfg := withResource(map[string]interface{}{
			"kind":     "Namespace",
			"metadata": map[string]interface{}{"name": "test"},
		})
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required Kubernetes field \"apiVersion\"")
	})

	t.Run("missing kind in manifest", func(t *testing.T) {
		cfg := withResource(map[string]interface{}{
			"apiVersion": "v1",
			"metadata":   map[string]interface{}{"name": "test"},
		})
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required Kubernetes field \"kind\"")
	})

	t.Run("missing metadata in manifest", func(t *testing.T) {
		cfg := withResource(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
		})
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required Kubernetes field \"metadata\"")
	})

	t.Run("missing name in metadata", func(t *testing.T) {
		cfg := withResource(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]interface{}{"labels": map[string]interface{}{"app": "test"}},
		})
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field \"name\"")
	})
}

func TestValidationErrorsFormat(t *testing.T) {
	errors := &ValidationErrors{}
	errors.Add("path.to.field", "some error message")
	errors.Add("another.path", "another error")

	assert.True(t, errors.HasErrors())
	assert.Len(t, errors.Errors, 2)
	assert.Contains(t, errors.Error(), "validation failed with 2 error(s)")
	assert.Contains(t, errors.Error(), "path.to.field: some error message")
	assert.Contains(t, errors.Error(), "another.path: another error")
}

func TestValidate(t *testing.T) {
	// Test that Validate catches multiple errors
	cfg := baseConfig()
	cfg.Spec.Steps = []Step{
		{Name: "check1", When: "invalid ))) syntax", Log: &LogStep{Message: "test"}},
		{
			Name: "testNs",
			Resource: &ResourceStep{
				Manifest: map[string]interface{}{
					"kind":     "Namespace", // missing apiVersion
					"metadata": map[string]interface{}{"name": "test"},
				},
				Discovery: &DiscoveryConfig{Namespace: "*", ByName: "test"},
			},
		},
	}

	err := newValidator(cfg).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestBuiltinVariables(t *testing.T) {
	// Test that builtin variables (like metadata.name) are recognized
	cfg := baseConfig()
	cfg.Spec.Steps = []Step{{
		Name: "testNs",
		Resource: &ResourceStep{
			Manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name":   "ns-{{ .metadata.name }}",
					"labels": map[string]interface{}{"adapter": "{{ .metadata.name }}"},
				},
			},
			Discovery: &DiscoveryConfig{Namespace: "*", ByName: "ns-{{ .metadata.name }}"},
		},
	}}
	assert.NoError(t, newValidator(cfg).Validate())
}

func TestValidateCaptureFields(t *testing.T) {
	// Helper to create config with capture fields
	withCapture := func(captures []CaptureField) *AdapterConfig {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{{
			Name:    "getStatus",
			APICall: &APICallStep{Method: "GET", URL: "http://example.com/api", Capture: captures},
		}}
		return cfg
	}

	t.Run("valid capture with field only", func(t *testing.T) {
		cfg := withCapture([]CaptureField{
			{Name: "clusterName", Field: "metadata.name"},
			{Name: "clusterPhase", Field: "status.phase"},
		})
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("valid capture with expression only", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{Name: "activeCount", Expression: "1 + 1"}})
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("invalid - both field and expression set", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{Name: "conflicting", Field: "metadata.name", Expression: "1 + 1"}})
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot have both 'field' and 'expression' set")
	})

	t.Run("invalid - neither field nor expression set", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{Name: "empty"}})
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must have either 'field' or 'expression' set")
	})

	t.Run("invalid - capture name missing", func(t *testing.T) {
		cfg := withCapture([]CaptureField{{Field: "metadata.name"}})
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "capture name is required")
	})
}

func TestValidatePayloadSteps(t *testing.T) {
	t.Run("valid payload step", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "clusterId", Param: &ParamStep{Value: "test-123"}},
			{
				Name: "statusPayload",
				Payload: map[string]interface{}{
					"clusterId": "{{ .clusterId }}",
					"status":    "ready",
				},
			},
		}
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("payload with nested expression", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "statusValue", Param: &ParamStep{Value: "healthy"}},
			{
				Name: "statusPayload",
				Payload: map[string]interface{}{
					"conditions": map[string]interface{}{
						"health": map[string]interface{}{
							"status": map[string]interface{}{
								"expression": `statusValue == "healthy"`,
							},
						},
					},
				},
			},
		}
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("payload with invalid CEL expression", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{{
			Name: "statusPayload",
			Payload: map[string]interface{}{
				"status": map[string]interface{}{
					"expression": `invalid ))) syntax`,
				},
			},
		}}
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CEL parse error")
	})
}

func TestValidateLogSteps(t *testing.T) {
	t.Run("valid log step", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "clusterId", Param: &ParamStep{Value: "test-123"}},
			{Name: "logStatus", Log: &LogStep{Level: "info", Message: "Processing cluster {{ .clusterId }}"}},
		}
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("log step with undefined variable", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{
			{Name: "logStatus", Log: &LogStep{Message: "Processing {{ .undefinedVar }}"}},
		}
		err := newValidator(cfg).Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefined template variable \"undefinedVar\"")
	})
}

func TestValidateEmptyConfig(t *testing.T) {
	t.Run("config with no steps is valid", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Spec.Steps = []Step{}
		assert.NoError(t, newValidator(cfg).Validate())
	})

	t.Run("nil config returns error", func(t *testing.T) {
		v := newValidator(nil)
		err := v.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "config is nil")
	})
}
