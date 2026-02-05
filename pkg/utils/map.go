package utils

import (
	"fmt"

	"github.com/mitchellh/copystructure"
)

// ConvertToStringKeyMap converts map[interface{}]interface{} to map[string]interface{}.
// This is commonly needed when working with YAML data which may have interface{} keys.
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

// convertSlice converts slice elements recursively, handling nested maps.
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

// DeepCopyMap creates a deep copy of a map.
// This handles non-JSON-serializable types and preserves type information
// (e.g., int64 stays int64, not float64 like JSON marshal/unmarshal).
//
// Returns an error if deep copy fails.
func DeepCopyMap(m map[string]interface{}) (map[string]interface{}, error) {
	if m == nil {
		return nil, nil
	}

	copied, err := copystructure.Copy(m)
	if err != nil {
		return nil, fmt.Errorf("failed to deep copy map: %w", err)
	}

	result, ok := copied.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected type after deep copy: %T", copied)
	}

	return result, nil
}

// DeepCopyMapWithFallback creates a deep copy of a map, falling back to shallow copy on error.
// Use this when you want to continue even if deep copy fails.
func DeepCopyMapWithFallback(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}

	result, err := DeepCopyMap(m)
	if err != nil {
		// Fallback to shallow copy
		result = make(map[string]interface{})
		for k, v := range m {
			result[k] = v
		}
	}

	return result
}

// GetNestedValue extracts a value from a nested map using dot notation.
// Supports both map[string]interface{} and map[interface{}]interface{} at any level.
//
// Parameters:
//   - data: The map to extract from
//   - path: Dot-separated path (e.g., "metadata.labels.app")
//
// Returns the value at the path or an error if any part of the path doesn't exist.
//
// Example:
//
//	data := map[string]interface{}{
//	    "metadata": map[string]interface{}{
//	        "labels": map[string]interface{}{
//	            "app": "myapp",
//	        },
//	    },
//	}
//	val, err := GetNestedValue(data, "metadata.labels.app")  // val = "myapp"
func GetNestedValue(data map[string]interface{}, path string) (interface{}, error) {
	parts := splitPath(path)
	var current interface{} = data

	for i, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("field '%s' not found at path '%s'", part, joinPath(parts[:i+1]))
			}
			current = val
		case map[interface{}]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("field '%s' not found at path '%s'", part, joinPath(parts[:i+1]))
			}
			current = val
		default:
			return nil, fmt.Errorf("cannot access field '%s': parent is not a map (got %T)", part, current)
		}
	}

	return current, nil
}

// splitPath splits a dot-separated path into parts.
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	// Simple split - doesn't handle escaped dots
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			if i > start {
				result = append(result, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		result = append(result, path[start:])
	}
	return result
}

// joinPath joins path parts with dots.
func joinPath(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "." + parts[i]
	}
	return result
}
