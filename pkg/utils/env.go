package utils

import (
	"fmt"
	"os"
)

// GetEnvOrError returns the value of an environment variable or an error if not set.
func GetEnvOrError(envVar string) (string, error) {
	value, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("environment variable %s not set", envVar)
	}
	return value, nil
}
