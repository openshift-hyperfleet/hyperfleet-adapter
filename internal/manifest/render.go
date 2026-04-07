// Package manifest provides utilities for Kubernetes manifest rendering and processing.
package manifest

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
	"gopkg.in/yaml.v3"
)

// ToYAMLString converts a manifest value to a YAML string.
// String values are returned as-is. Map values are marshaled to YAML.
func ToYAMLString(manifest interface{}) (string, error) {
	switch m := manifest.(type) {
	case string:
		return m, nil
	case map[string]interface{}:
		data, err := yaml.Marshal(m)
		if err != nil {
			return "", fmt.Errorf("failed to marshal manifest to YAML: %w", err)
		}
		return string(data), nil
	case map[interface{}]interface{}:
		converted := utils.ConvertToStringKeyMap(m)
		data, err := yaml.Marshal(converted)
		if err != nil {
			return "", fmt.Errorf("failed to marshal manifest to YAML: %w", err)
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported manifest type: %T", manifest)
	}
}

// RenderStringManifest renders a raw string manifest by executing Go templates,
// then parsing the result as YAML and marshaling to JSON bytes.
func RenderStringManifest(manifestStr string, params map[string]interface{}) ([]byte, error) {
	if strings.TrimSpace(manifestStr) == "" {
		return nil, fmt.Errorf("empty manifest: string manifest cannot be empty")
	}

	rendered, err := utils.RenderTemplate(manifestStr, params)
	if err != nil {
		return nil, fmt.Errorf("failed to render manifest template: %w", err)
	}

	if strings.TrimSpace(rendered) == "" {
		return nil, fmt.Errorf("empty manifest: template rendered to an empty document")
	}

	var manifestData map[string]interface{}
	if unmarshalErr := yaml.Unmarshal([]byte(rendered), &manifestData); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse rendered manifest as YAML: %w", unmarshalErr)
	}
	if len(manifestData) == 0 {
		return nil, fmt.Errorf("empty manifest: rendered YAML did not contain an object")
	}

	data, err := json.Marshal(manifestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rendered manifest: %w", err)
	}

	return data, nil
}
