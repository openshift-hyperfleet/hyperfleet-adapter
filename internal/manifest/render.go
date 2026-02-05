// Package manifest provides utilities for Kubernetes manifest rendering and processing.
package manifest

import (
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
)

// RenderManifestData recursively renders all template strings in a manifest data map.
// Both keys and values are rendered if they contain template expressions.
//
// Parameters:
//   - data: The manifest data as a map
//   - params: The parameters to use for template rendering
//
// Returns the rendered manifest data or an error if any template fails to render.
func RenderManifestData(data map[string]interface{}, params map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for k, v := range data {
		renderedKey, err := utils.RenderTemplate(k, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render key '%s': %w", k, err)
		}

		renderedValue, err := renderValue(v, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render value for key '%s': %w", k, err)
		}

		result[renderedKey] = renderedValue
	}

	return result, nil
}

// renderValue renders a value recursively, handling maps, slices, and strings.
func renderValue(v interface{}, params map[string]interface{}) (interface{}, error) {
	switch val := v.(type) {
	case string:
		return utils.RenderTemplate(val, params)
	case map[string]interface{}:
		return RenderManifestData(val, params)
	case map[interface{}]interface{}:
		converted := utils.ConvertToStringKeyMap(val)
		return RenderManifestData(converted, params)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			rendered, err := renderValue(item, params)
			if err != nil {
				return nil, err
			}
			result[i] = rendered
		}
		return result, nil
	default:
		return v, nil
	}
}
