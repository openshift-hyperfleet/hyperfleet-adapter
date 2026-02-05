// Package utils provides general-purpose utility functions.
package utils

import (
	"fmt"
	"os"
)

// GetEnvOrError retrieves an environment variable value.
// Unlike os.Getenv which returns an empty string for unset variables,
// this function returns an error if the variable is not set.
//
// Example:
//
//	val, err := GetEnvOrError("DATABASE_URL")
//	if err != nil {
//	    // Variable not set
//	}
func GetEnvOrError(envVar string) (string, error) {
	value, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("environment variable %s not set", envVar)
	}
	return value, nil
}
