package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/mitchellh/copystructure"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

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

// DeepCopyMap creates a deep copy of a map using github.com/mitchellh/copystructure.
func DeepCopyMap(ctx context.Context, m map[string]interface{}, log logger.Logger) map[string]interface{} {
	if m == nil {
		return nil
	}
	return DeepCopyMapWithFallback(ctx, m, log)
}

// DeepCopyMapWithFallback creates a deep copy with shallow copy fallback on error.
func DeepCopyMapWithFallback(ctx context.Context, m map[string]interface{}, log logger.Logger) map[string]interface{} {
	if m == nil {
		return nil
	}

	copied, err := copystructure.Copy(m)
	if err != nil {
		log.Warnf(ctx, "Failed to deep copy map: %v. Falling back to shallow copy.", err)
		result := make(map[string]interface{})
		for k, v := range m {
			result[k] = v
		}
		return result
	}

	result, ok := copied.(map[string]interface{})
	if !ok {
		result := make(map[string]interface{})
		for k, v := range m {
			result[k] = v
		}
		return result
	}

	return result
}

// GetNestedValue retrieves a nested value from a map using dot-separated path.
func GetNestedValue(m map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	var current interface{} = m

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, false
			}
			current = val
		default:
			return nil, false
		}
	}

	return current, true
}
