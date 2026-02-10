package manifest

import (
	"fmt"
)

// RenderManifestData recursively renders all template strings in a manifest data map.
// Keys and string values are rendered using the provided render function.
func RenderManifestData(data map[string]interface{}, renderFn func(string, map[string]interface{}) (string, error), params map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for k, v := range data {
		renderedKey, err := renderFn(k, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render key '%s': %w", k, err)
		}

		renderedValue, err := renderManifestValue(v, renderFn, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render value for key '%s': %w", k, err)
		}

		result[renderedKey] = renderedValue
	}

	return result, nil
}

// renderManifestValue renders a value recursively
func renderManifestValue(v interface{}, renderFn func(string, map[string]interface{}) (string, error), params map[string]interface{}) (interface{}, error) {
	switch val := v.(type) {
	case string:
		return renderFn(val, params)
	case map[string]interface{}:
		return RenderManifestData(val, renderFn, params)
	case map[interface{}]interface{}:
		converted := ConvertToStringKeyMap(val)
		return RenderManifestData(converted, renderFn, params)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			rendered, err := renderManifestValue(item, renderFn, params)
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

// ConvertToStringKeyMap converts map[interface{}]interface{} to map[string]interface{}
func ConvertToStringKeyMap(m map[interface{}]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		strKey := fmt.Sprintf("%v", k)
		switch val := v.(type) {
		case map[interface{}]interface{}:
			result[strKey] = ConvertToStringKeyMap(val)
		case []interface{}:
			result[strKey] = convertSlice(val)
		default:
			result[strKey] = v
		}
	}
	return result
}

// convertSlice converts slice elements recursively
func convertSlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[interface{}]interface{}:
			result[i] = ConvertToStringKeyMap(val)
		case []interface{}:
			result[i] = convertSlice(val)
		default:
			result[i] = v
		}
	}
	return result
}
