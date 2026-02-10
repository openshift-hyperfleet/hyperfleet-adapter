// Package utils provides general-purpose utility functions.
package utils

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ConvertToType converts a value to the specified type.
// Supported types: string, int, int64, float, float64, bool
//
// Example:
//
//	val, err := ConvertToType("42", "int64")  // val = int64(42)
//	val, err := ConvertToType(3.14, "string") // val = "3.14"
func ConvertToType(value interface{}, targetType string) (interface{}, error) {
	switch targetType {
	case "string":
		return ConvertToString(value)
	case "int", "int64":
		return ConvertToInt64(value)
	case "float", "float64":
		return ConvertToFloat64(value)
	case "bool":
		return ConvertToBool(value)
	default:
		return nil, fmt.Errorf("unsupported type: %s (supported: string, int, int64, float, float64, bool)", targetType)
	}
}

// ConvertToString converts a value to string.
// Handles all common types including numbers and booleans.
//
//nolint:unparam // error kept for API consistency
func ConvertToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// ConvertToInt64 converts a value to int64.
// Handles integers, floats, strings, and booleans.
//
// Example:
//
//	ConvertToInt64("42")     // int64(42), nil
//	ConvertToInt64(3.14)     // int64(3), nil
//	ConvertToInt64(true)     // int64(1), nil
func ConvertToInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("uint64 value %d overflows int64", v)
		}
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		if v > uint(math.MaxInt64) {
			return 0, fmt.Errorf("uint value %d overflows int64", v)
		}
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		// Try parsing as int first
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i, nil
		}
		// Try parsing as float and convert
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f), nil
		}
		return 0, fmt.Errorf("cannot convert string '%s' to int", v)
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", value)
	}
}

// ConvertToFloat64 converts a value to float64.
// Handles integers, floats, strings, and booleans.
//
// Example:
//
//	ConvertToFloat64("3.14")  // float64(3.14), nil
//	ConvertToFloat64(42)      // float64(42), nil
//	ConvertToFloat64(true)    // float64(1.0), nil
func ConvertToFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string '%s' to float: %w", v, err)
		}
		return f, nil
	case bool:
		if v {
			return 1.0, nil
		}
		return 0.0, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float", value)
	}
}

// ConvertToBool converts a value to bool.
// Handles booleans, strings (including "yes", "no", "on", "off"), and numbers.
//
// Example:
//
//	ConvertToBool("yes")   // true, nil
//	ConvertToBool("false") // false, nil
//	ConvertToBool(1)       // true, nil
//	ConvertToBool(0)       // false, nil
func ConvertToBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		// Empty string is treated as false
		if v == "" {
			return false, nil
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			// Handle common truthy/falsy strings
			lower := strings.ToLower(v)
			switch lower {
			case "yes", "y", "on", "1":
				return true, nil
			case "no", "n", "off", "0":
				return false, nil
			}
			return false, fmt.Errorf("cannot convert string '%s' to bool", v)
		}
		return b, nil
	// NOTE: Each numeric type needs its own case arm. In Go type switches, combined
	// cases like "case int, int8, int16:" keep v as interface{}, so "v != 0" would
	// compare interface{}(int8(0)) with interface{}(int(0)) - different types that
	// are never equal, causing int8(0) to incorrectly return true.
	// With separate arms, v is bound to the concrete type, enabling correct comparison.
	case int:
		return v != 0, nil
	case int8:
		return v != 0, nil
	case int16:
		return v != 0, nil
	case int32:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case uint:
		return v != 0, nil
	case uint8:
		return v != 0, nil
	case uint16:
		return v != 0, nil
	case uint32:
		return v != 0, nil
	case uint64:
		return v != 0, nil
	case float32:
		return v != 0, nil
	case float64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", value)
	}
}
