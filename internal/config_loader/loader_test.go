package config_loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "adapter-config.yaml")

	configYAML := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
  namespace: hyperfleet-system
  labels:
    hyperfleet.io/adapter-type: test
spec:
  adapter:
    version: "0.1.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 2s
    retryAttempts: 3
    retryBackoff: exponential
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "clusterId"
      param:
        source: "event.cluster_id"
    - name: "checkCluster"
      apiCall:
        method: "GET"
        url: "https://api.example.com/clusters/{{ .clusterId }}"
`

	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	require.NoError(t, err)

	// Test loading
	config, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify basic fields
	assert.Equal(t, "hyperfleet.redhat.com/v1alpha1", config.APIVersion)
	assert.Equal(t, "AdapterConfig", config.Kind)
	assert.Equal(t, "test-adapter", config.Metadata.Name)
	assert.Equal(t, "hyperfleet-system", config.Metadata.Namespace)
	assert.Equal(t, "0.1.0", config.Spec.Adapter.Version)
	assert.Len(t, config.Spec.Steps, 2)
}

func TestLoadInvalidPath(t *testing.T) {
	config, err := Load("/nonexistent/path/to/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid minimal config",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
`,
			wantError: false,
		},
		{
			name: "missing apiVersion",
			yaml: `
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
`,
			wantError: true,
			errorMsg:  "apiVersion is required",
		},
		{
			name: "missing kind",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
`,
			wantError: true,
			errorMsg:  "kind is required",
		},
		{
			name: "missing metadata.name",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  namespace: test
spec:
  adapter:
    version: "1.0.0"
`,
			wantError: true,
			errorMsg:  "metadata.name is required",
		},
		{
			name: "missing adapter.version",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter: {}
`,
			wantError: true,
			errorMsg:  "spec.adapter.version is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := Parse([]byte(tt.yaml))
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

func TestValidateSteps(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid param step with source",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "clusterId"
      param:
        source: "event.cluster_id"
`,
			wantError: false,
		},
		{
			name: "valid param step with value",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "version"
      param:
        value: "1.0.0"
`,
			wantError: false,
		},
		{
			name: "valid param step with expression",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "clusterUrl"
      param:
        expression: "'https://api.example.com/clusters/' + clusterId"
`,
			wantError: false,
		},
		{
			name: "step without name",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - param:
        source: "event.cluster_id"
`,
			wantError: true,
			errorMsg:  "name is required",
		},
		{
			name: "step without step type",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "emptyStep"
`,
			wantError: true,
			errorMsg:  "must specify one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := Parse([]byte(tt.yaml))
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

func TestValidateAPICallSteps(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid apiCall step",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "checkCluster"
      apiCall:
        method: "GET"
        url: "https://api.example.com/clusters"
`,
			wantError: false,
		},
		{
			name: "apiCall without method",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "checkCluster"
      apiCall:
        url: "https://api.example.com/clusters"
`,
			wantError: true,
			errorMsg:  "method is required",
		},
		{
			name: "apiCall with invalid method",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "checkCluster"
      apiCall:
        method: "INVALID"
        url: "https://api.example.com/clusters"
`,
			wantError: true,
			errorMsg:  "invalid method",
		},
		{
			name: "apiCall without url",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "checkCluster"
      apiCall:
        method: "GET"
`,
			wantError: true,
			errorMsg:  "url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := Parse([]byte(tt.yaml))
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

func TestValidateResourceSteps(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid resource step with manifest and discovery",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "testNamespace"
      resource:
        manifest:
          apiVersion: v1
          kind: Namespace
          metadata:
            name: "test-ns"
        discovery:
          namespace: "*"
          byName: "test-ns"
`,
			wantError: false,
		},
		{
			name: "resource without manifest",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "testNamespace"
      resource:
        discovery:
          namespace: "*"
          byName: "test-ns"
`,
			wantError: true,
			errorMsg:  "manifest is required",
		},
		{
			name: "resource without discovery",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "testNamespace"
      resource:
        manifest:
          apiVersion: v1
          kind: Namespace
          metadata:
            name: test
`,
			wantError: true,
			errorMsg:  "discovery is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := Parse([]byte(tt.yaml))
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

func TestGetStepByName(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "step1"
      param:
        value: "value1"
    - name: "step2"
      param:
        value: "value2"
`

	config, err := Parse([]byte(yaml))
	require.NoError(t, err)

	step := config.GetStepByName("step1")
	assert.NotNil(t, step)
	assert.Equal(t, "step1", step.Name)

	step = config.GetStepByName("nonexistent")
	assert.Nil(t, step)
}

func TestStepNames(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "clusterId"
      param:
        source: "event.cluster_id"
    - name: "checkCluster"
      apiCall:
        method: "GET"
        url: "https://api.example.com/clusters"
    - name: "createResource"
      resource:
        manifest:
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: test
        discovery:
          byName: "test"
`

	config, err := Parse([]byte(yaml))
	require.NoError(t, err)

	names := config.StepNames()
	assert.Len(t, names, 3)
	assert.Contains(t, names, "clusterId")
	assert.Contains(t, names, "checkCluster")
	assert.Contains(t, names, "createResource")
}

func TestParseTimeout(t *testing.T) {
	config := &HyperfleetAPIConfig{
		Timeout: "5s",
	}

	duration, err := config.ParseTimeout()
	require.NoError(t, err)
	assert.Equal(t, "5s", duration.String())

	config.Timeout = "invalid"
	_, err = config.ParseTimeout()
	assert.Error(t, err)
}

func TestGetBaseURL(t *testing.T) {
	// Test: returns config value when set
	config := &HyperfleetAPIConfig{
		BaseURL: "https://api.example.com",
	}
	assert.Equal(t, "https://api.example.com", config.GetBaseURL())

	// Test: returns empty string when config value is not set
	// (does NOT read from env var - that's NewClient's responsibility)
	config = &HyperfleetAPIConfig{}
	assert.Equal(t, "", config.GetBaseURL())

	// Test: nil config returns empty string
	var nilConfig *HyperfleetAPIConfig
	assert.Equal(t, "", nilConfig.GetBaseURL())

	// Test: explicitly verify no env var fallback
	// Set env var and verify GetBaseURL still returns empty (not the env value)
	t.Setenv(hyperfleet_api.EnvBaseURL, "https://env-api.example.com")
	config = &HyperfleetAPIConfig{} // No BaseURL configured
	assert.Equal(t, "", config.GetBaseURL(), "GetBaseURL should NOT read from env var; env fallback is handled by hyperfleet_api.NewClient")
}

func TestUnsupportedAPIVersion(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v2
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
`
	config, err := Parse([]byte(yaml))
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "unsupported apiVersion")
	assert.Contains(t, err.Error(), "hyperfleet.redhat.com/v2")
}

func TestInvalidKind(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: WrongKind
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
`
	config, err := Parse([]byte(yaml))
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "invalid kind")
	assert.Contains(t, err.Error(), "WrongKind")
	assert.Contains(t, err.Error(), "AdapterConfig")
}

func TestValidateAdapterVersion(t *testing.T) {
	config := &AdapterConfig{
		Spec: AdapterConfigSpec{
			Adapter: AdapterInfo{
				Version: "1.0.0",
			},
		},
	}

	// Matching version
	err := ValidateAdapterVersion(config, "1.0.0")
	assert.NoError(t, err)

	// Mismatched version
	err = ValidateAdapterVersion(config, "2.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter version mismatch")

	// Empty expected version (skip validation)
	err = ValidateAdapterVersion(config, "")
	assert.NoError(t, err)
}

func TestIsSupportedAPIVersion(t *testing.T) {
	// Supported version
	assert.True(t, IsSupportedAPIVersion("hyperfleet.redhat.com/v1alpha1"))

	// Unsupported versions
	assert.False(t, IsSupportedAPIVersion("hyperfleet.redhat.com/v1"))
	assert.False(t, IsSupportedAPIVersion("hyperfleet.redhat.com/v2"))
	assert.False(t, IsSupportedAPIVersion("other.io/v1alpha1"))
	assert.False(t, IsSupportedAPIVersion(""))
}

func TestSupportedAPIVersions(t *testing.T) {
	// Verify the constant is in the supported list
	assert.Contains(t, SupportedAPIVersions, APIVersionV1Alpha1)
	assert.Equal(t, "hyperfleet.redhat.com/v1alpha1", APIVersionV1Alpha1)
}

func TestValidateResourceDiscovery(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - resource step with discovery bySelectors",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "test"
      resource:
        manifest:
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: test
        discovery:
          namespace: "test-ns"
          bySelectors:
            labelSelector:
              app: "test"
`,
			wantErr: false,
		},
		{
			name: "valid - resource step with discovery byName",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "test"
      resource:
        manifest:
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: test
        discovery:
          namespace: "*"
          byName: "my-resource"
`,
			wantErr: false,
		},
		{
			name: "invalid - resource step missing byName or bySelectors",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "test"
      resource:
        manifest:
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: test
        discovery:
          namespace: "test-ns"
`,
			wantErr: true,
			errMsg:  "must have either byName or bySelectors",
		},
		{
			name: "invalid - bySelectors without labelSelector defined",
			yaml: `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "test"
      resource:
        manifest:
          apiVersion: v1
          kind: ConfigMap
          metadata:
            name: test
        discovery:
          namespace: "test-ns"
          bySelectors: {}
`,
			wantErr: true,
			errMsg:  "must have labelSelector defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLogStep(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "logInfo"
      log:
        level: "info"
        message: "Processing complete"
`

	config, err := Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, config.Spec.Steps, 1)

	step := config.Spec.Steps[0]
	assert.Equal(t, "logInfo", step.Name)
	assert.NotNil(t, step.Log)
	assert.Equal(t, "info", step.Log.Level)
	assert.Equal(t, "Processing complete", step.Log.Message)
}

func TestPayloadStep(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "statusPayload"
      payload:
        status: "ready"
        timestamp: "2024-01-01"
`

	config, err := Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, config.Spec.Steps, 1)

	step := config.Spec.Steps[0]
	assert.Equal(t, "statusPayload", step.Name)
	assert.NotNil(t, step.Payload)

	payload, ok := step.Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ready", payload["status"])
	assert.Equal(t, "2024-01-01", payload["timestamp"])
}

func TestWhenClause(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "status"
      param:
        value: "active"
    - name: "conditionalLog"
      when: "status == 'active'"
      log:
        message: "Status is active"
`

	config, err := Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, config.Spec.Steps, 2)

	// Check when clause
	assert.Equal(t, "status == 'active'", config.Spec.Steps[1].When)
}

func TestAPICallCapture(t *testing.T) {
	yaml := `
apiVersion: hyperfleet.redhat.com/v1alpha1
kind: AdapterConfig
metadata:
  name: test-adapter
spec:
  adapter:
    version: "1.0.0"
  hyperfleetApi:
    baseUrl: "https://test.example.com"
    timeout: 5s
  kubernetes:
    apiVersion: "v1"
  steps:
    - name: "getCluster"
      apiCall:
        method: "GET"
        url: "https://api.example.com/clusters/123"
        capture:
          - name: "clusterPhase"
            field: "status.phase"
          - name: "clusterName"
            field: "metadata.name"
`

	config, err := Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, config.Spec.Steps, 1)

	step := config.Spec.Steps[0]
	assert.NotNil(t, step.APICall)
	require.Len(t, step.APICall.Capture, 2)

	assert.Equal(t, "clusterPhase", step.APICall.Capture[0].Name)
	assert.Equal(t, "status.phase", step.APICall.Capture[0].Field)

	assert.Equal(t, "clusterName", step.APICall.Capture[1].Name)
	assert.Equal(t, "metadata.name", step.APICall.Capture[1].Field)
}
