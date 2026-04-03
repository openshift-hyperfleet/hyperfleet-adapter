package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8sclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transportclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestResourceExecutor_ExecuteAll_DiscoveryFailure verifies that when discovery fails after a successful apply,
// the error is logged and notified: ExecuteAll returns an error, result is failed,
// and execCtx.Adapter.ExecutionError is set.
func TestResourceExecutor_ExecuteAll_DiscoveryFailure(t *testing.T) {
	discoveryErr := errors.New("discovery failed: resource not found")
	mock := k8sclient.NewMockK8sClient()
	mock.GetResourceError = discoveryErr
	// Apply succeeds so we reach discovery
	mock.ApplyResourceResult = &transportclient.ApplyResult{
		Operation: manifest.OperationCreate,
		Reason:    "mock",
	}

	config := &ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	}
	re := newResourceExecutor(config)

	resource := configloader.Resource{
		Name:      "test-resource",
		Transport: &configloader.TransportConfig{Client: "kubernetes"},
		Manifest: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
		},
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-cm",
		},
	}
	resources := []configloader.Resource{resource}
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
	mock := k8sclient.NewMockK8sClient()
	mock.ApplyResourceResult = &transportclient.ApplyResult{
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

	resource := configloader.Resource{
		Name: "resource0",
		Transport: &configloader.TransportConfig{
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
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "cluster-1-adapter2",
		},
		NestedDiscoveries: []configloader.NestedDiscovery{
			{
				Name: "configmap0",
				Discovery: &configloader.DiscoveryConfig{
					Namespace: "default",
					ByName:    "cluster-1-adapter2-configmap",
				},
			},
		},
	}

	execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)
	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)
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

func TestRenderToBytes_StringManifest(t *testing.T) {
	re := newResourceExecutor(&ExecutorConfig{
		Logger: logger.NewTestLogger(),
	})

	tests := []struct {
		name         string
		manifest     string
		params       map[string]interface{}
		wantContains []string
		wantErr      bool
	}{
		{
			name: "simple string manifest with template values",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .name }}"
  namespace: "{{ .namespace }}"
data:
  key: value`,
			params: map[string]interface{}{
				"name":      "my-config",
				"namespace": "default",
			},
			wantContains: []string{`"name":"my-config"`, `"namespace":"default"`},
		},
		{
			name: "structural if template",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
{{ if .addLabels }}
  labels:
    app: "myapp"
{{ end }}
data:
  key: value`,
			params: map[string]interface{}{
				"addLabels": true,
			},
			wantContains: []string{`"labels"`, `"app":"myapp"`},
		},
		{
			name: "structural if template - false branch",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
{{ if .addLabels }}
  labels:
    app: "myapp"
{{ end }}
data:
  key: value`,
			params: map[string]interface{}{
				"addLabels": false,
			},
			wantContains: []string{`"name":"test"`, `"key":"value"`},
		},
		{
			name: "range template for list generation",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
data:
{{ range $k, $v := .items }}
  {{ $k }}: "{{ $v }}"
{{ end }}`,
			params: map[string]interface{}{
				"items": map[string]interface{}{
					"key1": "val1",
					"key2": "val2",
				},
			},
			wantContains: []string{`"key1":"val1"`, `"key2":"val2"`},
		},
		{
			name: "if-else template for conditional properties",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: "test"
  labels:
{{ if .isGood }}
    status: "good"
{{ else }}
    status: "bad"
{{ end }}`,
			params: map[string]interface{}{
				"isGood": true,
			},
			wantContains: []string{`"status":"good"`},
		},
		{
			name:     "invalid template syntax",
			manifest: `apiVersion: v1{{ if }}`,
			params:   map[string]interface{}{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := configloader.Resource{
				Name:     "test",
				Manifest: tt.manifest,
			}
			execCtx := NewExecutionContext(context.Background(), nil, nil)
			execCtx.Params = tt.params

			data, err := re.renderToBytes(resource, execCtx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			for _, want := range tt.wantContains {
				assert.Contains(t, string(data), want)
			}
		})
	}
}

func TestRenderToBytes_StringManifestWithSubnetList(t *testing.T) {
	// Test the customer's original use case: generating a list of subnets
	re := newResourceExecutor(&ExecutorConfig{
		Logger: logger.NewTestLogger(),
	})

	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "subnet-config"
data:
  subnets: |
{{ range .subnetIds }}
    - id: {{ . }}
{{ end }}`

	params := map[string]interface{}{
		"subnetIds": []interface{}{"sub1", "sub2", "sub3"},
	}

	resource := configloader.Resource{
		Name:     "subnets",
		Manifest: manifest,
	}
	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params = params

	data, err := re.renderToBytes(resource, execCtx)
	require.NoError(t, err)
	assert.Contains(t, string(data), "sub1")
	assert.Contains(t, string(data), "sub2")
	assert.Contains(t, string(data), "sub3")
}

func TestRenderToBytes_StringManifestEdgeCases(t *testing.T) {
	re := newResourceExecutor(&ExecutorConfig{
		Logger: logger.NewTestLogger(),
	})

	t.Run("plain YAML string without templates", func(t *testing.T) {
		// Backward compatibility: plain YAML ref files (no templates) still work
		manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "static-config"
data:
  key: value`
		resource := configloader.Resource{
			Name:     "test",
			Manifest: manifest,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{}

		data, err := re.renderToBytes(resource, execCtx)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"static-config"`)
		assert.Contains(t, string(data), `"key":"value"`)
	})

	t.Run("empty string manifest", func(t *testing.T) {
		resource := configloader.Resource{
			Name:     "test",
			Manifest: "",
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{}

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty manifest")
	})

	t.Run("template rendering produces invalid YAML", func(t *testing.T) {
		manifest := `{{ .content }}`
		resource := configloader.Resource{
			Name:     "test",
			Manifest: manifest,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{
			"content": "not: valid: yaml: [broken",
		}

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse rendered manifest as YAML")
	})

	t.Run("missing template variable errors", func(t *testing.T) {
		manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .missingVar }}"`
		resource := configloader.Resource{
			Name:     "test",
			Manifest: manifest,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{} // missingVar not provided

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missingVar")
	})

	t.Run("nil manifest", func(t *testing.T) {
		resource := configloader.Resource{
			Name:     "test",
			Manifest: nil,
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)

		_, err := re.renderToBytes(resource, execCtx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no manifest specified")
	})

	t.Run("map manifest still works (backward compatibility)", func(t *testing.T) {
		resource := configloader.Resource{
			Name: "test",
			Manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "{{ .name }}",
					"namespace": "default",
				},
			},
		}
		execCtx := NewExecutionContext(context.Background(), nil, nil)
		execCtx.Params = map[string]interface{}{
			"name": "rendered-name",
		}

		data, err := re.renderToBytes(resource, execCtx)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"rendered-name"`)
		assert.Contains(t, string(data), `"namespace":"default"`)
	})
}

func TestResourceExecutor_ExecuteAll_StringManifest(t *testing.T) {
	// End-to-end test: string manifest through the full executor flow
	mock := k8sclient.NewMockK8sClient()
	// Don't set ApplyResourceResult — use default behavior which parses and stores the resource
	mock.GetResourceResult = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-config",
				"namespace": "default",
			},
		},
	}

	re := newResourceExecutor(&ExecutorConfig{
		TransportClient: mock,
		Logger:          logger.NewTestLogger(),
	})

	// Use a string manifest with structural Go templates
	manifestStr := `apiVersion: v1
kind: ConfigMap
metadata:
  name: "{{ .configName }}"
  namespace: "{{ .namespace }}"
{{ if .addLabels }}
  labels:
    managed-by: "adapter"
{{ end }}
data:
  cluster: "{{ .clusterId }}"`

	resource := configloader.Resource{
		Name:     "testConfig",
		Manifest: manifestStr,
		Discovery: &configloader.DiscoveryConfig{
			Namespace: "default",
			ByName:    "test-config",
		},
	}

	execCtx := NewExecutionContext(context.Background(), nil, nil)
	execCtx.Params = map[string]interface{}{
		"configName": "test-config",
		"namespace":  "default",
		"addLabels":  true,
		"clusterId":  "cluster-1",
	}

	results, err := re.ExecuteAll(context.Background(), []configloader.Resource{resource}, execCtx)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, StatusSuccess, results[0].Status)
	assert.Equal(t, "ConfigMap", results[0].Kind)
	assert.Equal(t, "test-config", results[0].ResourceName)

	// Verify the mock stored the rendered resource correctly
	stored, ok := mock.Resources["default/test-config"]
	require.True(t, ok, "Resource should be stored in mock")
	assert.Equal(t, "ConfigMap", stored.GetKind())
	assert.Equal(t, "test-config", stored.GetName())

	// Verify labels were rendered (addLabels=true)
	labels := stored.GetLabels()
	assert.Equal(t, "adapter", labels["managed-by"])

	// Verify data was rendered
	data, found, _ := unstructured.NestedString(stored.Object, "data", "cluster")
	assert.True(t, found)
	assert.Equal(t, "cluster-1", data)
}

func TestResolveGVK_StringManifest(t *testing.T) {
	re := &ResourceExecutor{}

	tests := []struct {
		name        string
		manifest    interface{}
		wantGroup   string
		wantVersion string
		wantKind    string
		wantEmpty   bool
	}{
		{
			name: "map manifest",
			manifest: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
			wantVersion: "v1",
			wantKind:    "ConfigMap",
		},
		{
			name:        "string manifest with Go templates",
			manifest:    "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: \"{{ .clusterId }}\"\n",
			wantVersion: "v1",
			wantKind:    "ConfigMap",
		},
		{
			name: "string manifest with structural Go template directives",
			manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n" +
				"  name: \"test-{{ .clusterId }}\"\n  labels:\n    app: test\n" +
				"{{ if .testRunId }}\n    run-id: \"{{ .testRunId }}\"\n{{ end }}\n" +
				"data:\n  key: value\n",
			wantVersion: "v1",
			wantKind:    "ConfigMap",
		},
		{
			name:        "string manifest with apps/v1",
			manifest:    "apiVersion: apps/v1\nkind: Deployment\n",
			wantGroup:   "apps",
			wantVersion: "v1",
			wantKind:    "Deployment",
		},
		{
			name:      "nil manifest",
			manifest:  nil,
			wantEmpty: true,
		},
		{
			name:      "invalid string YAML",
			manifest:  "not: valid: yaml: {{{}",
			wantEmpty: true,
		},
		{
			name:      "string manifest missing kind",
			manifest:  "apiVersion: v1\nmetadata:\n  name: test\n",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := configloader.Resource{
				Manifest: tt.manifest,
			}
			gvk := re.resolveGVK(resource)

			if tt.wantEmpty {
				assert.True(t, gvk.Empty(), "expected empty GVK")
			} else {
				assert.Equal(t, tt.wantGroup, gvk.Group)
				assert.Equal(t, tt.wantVersion, gvk.Version)
				assert.Equal(t, tt.wantKind, gvk.Kind)
			}
		})
	}
}
