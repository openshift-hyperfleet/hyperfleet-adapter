package config_loader

// Step represents a single execution step in the step-based DSL.
// Each step has a unique name and executes one of: param, apiCall, resource, payload, or log.
// Steps execute sequentially in order. The optional 'when' clause (CEL expression)
// controls whether the step runs - if false, the step is skipped (soft failure).
type Step struct {
	Name     string        `yaml:"name"`
	When     string        `yaml:"when,omitempty"`     // CEL expression - if false, step is skipped
	Param    *ParamStep    `yaml:"param,omitempty"`    // Parameter definition
	APICall  *APICallStep  `yaml:"apiCall,omitempty"`  // HTTP API call
	Resource *ResourceStep `yaml:"resource,omitempty"` // Kubernetes resource
	Payload  interface{}   `yaml:"payload,omitempty"`  // Payload builder (flexible structure)
	Log      *LogStep      `yaml:"log,omitempty"`      // Log message
}

// ParamStep defines a parameter step that extracts or computes a value.
// Exactly one of Source, Value, or Expression should be set.
// If the source/expression resolves to nothing, Default is used.
//
// Examples:
//
//	# From environment variable
//	param:
//	  source: "env.HYPERFLEET_API_URL"
//
//	# From event data
//	param:
//	  source: "event.cluster_id"
//
//	# Literal value
//	param:
//	  value: "0.1.0"
//
//	# Computed via CEL expression
//	param:
//	  expression: "hyperfleetApiBaseUrl + '/clusters/' + clusterId"
type ParamStep struct {
	Source     string      `yaml:"source,omitempty"`     // Extract from: env.*, event.*
	Value      interface{} `yaml:"value,omitempty"`      // Literal value
	Expression string      `yaml:"expression,omitempty"` // CEL expression
	Default    interface{} `yaml:"default,omitempty"`    // Default if source/expression resolves to nothing
}

// APICallStep defines an HTTP API call step.
// The response is stored as the step result and can be accessed by step name.
// Optional captures extract fields from the response to top-level variables.
//
// Example:
//
//	apiCall:
//	  method: GET
//	  url: "{{ .hyperfleetApiBaseUrl }}/clusters/{{ .clusterId }}"
//	  timeout: 10s
//	  capture:
//	    - name: "clusterPhase"
//	      field: "status.phase"
type APICallStep struct {
	Method        string         `yaml:"method"`
	URL           string         `yaml:"url"`
	Timeout       string         `yaml:"timeout,omitempty"`
	RetryAttempts int            `yaml:"retryAttempts,omitempty"`
	RetryBackoff  string         `yaml:"retryBackoff,omitempty"`
	Headers       []Header       `yaml:"headers,omitempty"`
	Body          string         `yaml:"body,omitempty"`
	Capture       []CaptureField `yaml:"capture,omitempty"` // Fields to capture from response
}

// ResourceStep defines a Kubernetes resource step.
// Creates or updates a K8s resource based on the manifest.
// The resulting resource is stored as the step result.
//
// Example:
//
//	resource:
//	  manifest:
//	    apiVersion: v1
//	    kind: Namespace
//	    metadata:
//	      name: "{{ .clusterId | lower }}"
//	  discovery:
//	    byName: "{{ .clusterId | lower }}"
type ResourceStep struct {
	Manifest         interface{}      `yaml:"manifest"`
	Discovery        *DiscoveryConfig `yaml:"discovery,omitempty"`
	RecreateOnChange bool             `yaml:"recreateOnChange,omitempty"`
}

// LogStep defines a logging step.
// Emits a log message at the specified level.
//
// Example:
//
//	log:
//	  level: info
//	  message: "Processing cluster {{ .clusterId }}"
type LogStep struct {
	Level   string `yaml:"level,omitempty"` // debug, info, warn, error (default: info)
	Message string `yaml:"message"`
}

// GetStepType returns the type of the step based on which field is set.
// Returns empty string if no step type is set.
func (s *Step) GetStepType() string {
	switch {
	case s.Param != nil:
		return "param"
	case s.APICall != nil:
		return "apiCall"
	case s.Resource != nil:
		return "resource"
	case s.Payload != nil:
		return "payload"
	case s.Log != nil:
		return "log"
	default:
		return ""
	}
}

// CountStepTypes returns the number of step type fields that are set.
// Valid steps should have exactly 1.
func (s *Step) CountStepTypes() int {
	count := 0
	if s.Param != nil {
		count++
	}
	if s.APICall != nil {
		count++
	}
	if s.Resource != nil {
		count++
	}
	if s.Payload != nil {
		count++
	}
	if s.Log != nil {
		count++
	}
	return count
}
