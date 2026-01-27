package config_loader

import (
	"fmt"
	"time"
)

// -----------------------------------------------------------------------------
// Built-in Variables
// -----------------------------------------------------------------------------

// builtinVariables is the list of built-in variables always available in templates/CEL
var builtinVariables = []string{
	"metadata", "metadata.name", "metadata.namespace", "metadata.labels",
	"now", "date",
}

// BuiltinVariables returns the list of built-in variables always available in templates/CEL
func BuiltinVariables() []string {
	return builtinVariables
}

// -----------------------------------------------------------------------------
// AdapterConfig Accessors
// -----------------------------------------------------------------------------

// GetDefinedVariables returns all variables defined in the config that can be used
// in templates and CEL expressions. This includes:
// - Built-in variables (metadata, now, date)
// - Step names (each step's result is accessible by name)
// - Captured variables from API call steps
func (c *AdapterConfig) GetDefinedVariables() map[string]bool {
	vars := make(map[string]bool)

	if c == nil {
		return vars
	}

	// Built-in variables
	for _, b := range BuiltinVariables() {
		vars[b] = true
	}

	// Variables from steps
	for _, step := range c.Spec.Steps {
		if step.Name != "" {
			vars[step.Name] = true
		}
		// Captures from API call steps
		if step.APICall != nil {
			for _, capture := range step.APICall.Capture {
				if capture.Name != "" {
					vars[capture.Name] = true
				}
			}
		}
	}

	return vars
}

// GetStepByName returns a step by name, or nil if not found
func (c *AdapterConfig) GetStepByName(name string) *Step {
	if c == nil {
		return nil
	}
	for i := range c.Spec.Steps {
		if c.Spec.Steps[i].Name == name {
			return &c.Spec.Steps[i]
		}
	}
	return nil
}

// StepNames returns all step names in order
func (c *AdapterConfig) StepNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, len(c.Spec.Steps))
	for i, s := range c.Spec.Steps {
		names[i] = s.Name
	}
	return names
}

// -----------------------------------------------------------------------------
// HyperfleetAPIConfig Accessors
// -----------------------------------------------------------------------------

// ParseTimeout parses the timeout string to time.Duration
// Returns 0 and nil if timeout is empty (caller should use default)
func (c *HyperfleetAPIConfig) ParseTimeout() (time.Duration, error) {
	if c == nil || c.Timeout == "" {
		return 0, nil
	}
	return time.ParseDuration(c.Timeout)
}

// GetBaseURL returns the base URL configured in HyperfleetAPIConfig.
// Returns empty string if BaseURL is not set in the config.
//
// Note: This method only returns the explicitly configured value. Environment
// variable fallback (HYPERFLEET_API_BASE_URL) is handled by hyperfleet_api.NewClient
// as a last resort when no base URL is provided via options.
func (c *HyperfleetAPIConfig) GetBaseURL() string {
	if c != nil && c.BaseURL != "" {
		return c.BaseURL
	}
	return ""
}

// -----------------------------------------------------------------------------
// Resource Accessors
// -----------------------------------------------------------------------------

// HasManifestRef returns true if the manifest uses a ref (single file reference)
func (r *Resource) HasManifestRef() bool {
	if r == nil || r.Manifest == nil {
		return false
	}
	manifest := normalizeToStringKeyMap(r.Manifest)
	if manifest == nil {
		return false
	}
	_, hasRef := manifest["ref"]
	return hasRef
}

// GetManifestRef returns the ref path if set, empty string otherwise
func (r *Resource) GetManifestRef() string {
	if r == nil || r.Manifest == nil {
		return ""
	}
	manifest := normalizeToStringKeyMap(r.Manifest)
	if manifest == nil {
		return ""
	}

	if ref, ok := manifest["ref"].(string); ok {
		return ref
	}

	return ""
}

// UnmarshalManifest attempts to unmarshal the manifest as a map
// Returns nil, nil if resource is nil or manifest is nil
// Returns error if manifest cannot be converted to map
func (r *Resource) UnmarshalManifest() (map[string]interface{}, error) {
	if r == nil || r.Manifest == nil {
		return nil, nil
	}

	// Try to normalize the manifest to map[string]interface{}
	if m := normalizeToStringKeyMap(r.Manifest); m != nil {
		return m, nil
	}

	// If manifest cannot be normalized, return an error with type info
	return nil, fmt.Errorf("manifest is not a map, got %T", r.Manifest)
}

// -----------------------------------------------------------------------------
// Helper Functions
// -----------------------------------------------------------------------------

// normalizeToStringKeyMap converts various map types to map[string]interface{}.
// This handles both map[string]interface{} (from yaml.v3) and map[interface{}]interface{}
// (from yaml.v2 or other sources) for robustness.
// Returns nil if the input is not a map type.
func normalizeToStringKeyMap(v interface{}) map[string]interface{} {
	switch m := v.(type) {
	case map[string]interface{}:
		return m
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(m))
		for k, val := range m {
			if keyStr, ok := k.(string); ok {
				result[keyStr] = val
			} else {
				// Convert non-string keys to string representation
				result[fmt.Sprintf("%v", k)] = val
			}
		}
		return result
	default:
		return nil
	}
}
