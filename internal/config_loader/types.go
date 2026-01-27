package config_loader

import (
	"gopkg.in/yaml.v3"
)

// ValueDef represents a dynamic value definition in payload builds.
// Used when a payload field should be computed via field extraction (JSONPath) or CEL expression.
// Only one of Field or Expression should be set.
//
// Example YAML with field (JSONPath):
//
//	status:
//	  field: "response.status"
//	  default: "unknown"
//
// Example YAML with expression (CEL):
//
//	status:
//	  expression: "adapter.?errorMessage.orValue(\"\")"
//	  default: "success"
type ValueDef struct {
	Field      string `yaml:"field"`      // JSONPath/dot notation to extract value
	Expression string `yaml:"expression"` // CEL expression to evaluate
	Default    any    `yaml:"default"`    // Default value if extraction fails or returns nil
}

// ParseValueDef attempts to parse a value as a ValueDef.
// Returns the parsed ValueDef and true if the value contains either field or expression.
// Returns nil and false if the value is not a value definition.
func ParseValueDef(v any) (*ValueDef, bool) {
	// Must be a map to be a value definition
	if _, ok := v.(map[string]any); !ok {
		return nil, false
	}

	// Marshal to YAML then unmarshal to ValueDef
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, false
	}

	var valueDef ValueDef
	if err := yaml.Unmarshal(data, &valueDef); err != nil {
		return nil, false
	}

	// Must have at least one of field or expression
	if valueDef.Field == "" && valueDef.Expression == "" {
		return nil, false
	}

	return &valueDef, true
}

// AdapterConfig represents the complete adapter configuration structure
type AdapterConfig struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   Metadata          `yaml:"metadata"`
	Spec       AdapterConfigSpec `yaml:"spec"`
}

// Metadata contains the adapter metadata
type Metadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

// AdapterConfigSpec contains the adapter specification.
// Uses step-based execution model where all operations are sequential steps with 'when' clauses.
type AdapterConfigSpec struct {
	Adapter       AdapterInfo         `yaml:"adapter"`
	HyperfleetAPI HyperfleetAPIConfig `yaml:"hyperfleetApi"`
	Kubernetes    KubernetesConfig    `yaml:"kubernetes"`

	// Steps defines the sequential execution steps
	Steps []Step `yaml:"steps,omitempty"`
}

// AdapterInfo contains basic adapter information
type AdapterInfo struct {
	Version string `yaml:"version"`
}

// HyperfleetAPIConfig contains HyperFleet API configuration
type HyperfleetAPIConfig struct {
	BaseURL       string `yaml:"baseUrl,omitempty"`
	Timeout       string `yaml:"timeout"`
	RetryAttempts int    `yaml:"retryAttempts"`
	RetryBackoff  string `yaml:"retryBackoff"`
}

// KubernetesConfig contains Kubernetes configuration
type KubernetesConfig struct {
	APIVersion string `yaml:"apiVersion"`
}


// Header represents an HTTP header
type Header struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// CaptureField represents a field capture configuration from API response.
//
// Supports two modes (mutually exclusive):
//   - Field: JSONPath expression for simple field extraction (e.g., "{.items[0].name}")
//   - Expression: CEL expression for complex transformations (e.g., "response.items.filter(i, i.adapter == 'x')")
type CaptureField struct {
	Name       string `yaml:"name"`
	Field      string `yaml:"field,omitempty"`
	Expression string `yaml:"expression,omitempty"`
}

// APICall represents an HTTP API call configuration.
// Used internally by the step executor for API calls.
type APICall struct {
	Method        string   `yaml:"method"`
	URL           string   `yaml:"url"`
	Timeout       string   `yaml:"timeout,omitempty"`
	RetryAttempts int      `yaml:"retryAttempts,omitempty"`
	RetryBackoff  string   `yaml:"retryBackoff,omitempty"`
	Headers       []Header `yaml:"headers,omitempty"`
	Body          string   `yaml:"body,omitempty"`
}

// Resource represents a Kubernetes resource configuration
type Resource struct {
	Name             string           `yaml:"name"`
	Manifest         interface{}      `yaml:"manifest,omitempty"`
	RecreateOnChange bool             `yaml:"recreateOnChange,omitempty"`
	Discovery        *DiscoveryConfig `yaml:"discovery,omitempty"`
}

// DiscoveryConfig represents resource discovery configuration
type DiscoveryConfig struct {
	Namespace   string          `yaml:"namespace,omitempty"`
	ByName      string          `yaml:"byName,omitempty"`
	BySelectors *SelectorConfig `yaml:"bySelectors,omitempty"`
}

// SelectorConfig represents label selector configuration
type SelectorConfig struct {
	LabelSelector map[string]string `yaml:"labelSelector,omitempty"`
}

