package utils

import "reflect"

// IsSliceOrArray returns true if the value is a slice or array.
func IsSliceOrArray(v interface{}) bool {
	if v == nil {
		return false
	}
	kind := reflect.TypeOf(v).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}
