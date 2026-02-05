// Package utils provides general-purpose utility functions.
package utils

import "reflect"

// IsSliceOrArray checks if a value is a slice or array using reflection.
// Returns false for nil values.
//
// Example:
//
//	IsSliceOrArray([]int{1, 2, 3})      // true
//	IsSliceOrArray([3]int{1, 2, 3})     // true
//	IsSliceOrArray("not a slice")       // false
//	IsSliceOrArray(nil)                 // false
func IsSliceOrArray(value interface{}) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}
