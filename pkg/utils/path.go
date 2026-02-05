// Package utils provides general-purpose utility functions.
package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveSecurePath resolves a relative path against the base directory and validates
// that the resolved path does not escape the base directory.
// This prevents directory traversal attacks (e.g., "../../../etc/passwd").
//
// Parameters:
//   - baseDir: The base directory to resolve paths relative to
//   - refPath: The path to resolve (can be relative or absolute)
//
// Returns the resolved absolute path or an error if the path escapes baseDir.
//
// Example:
//
//	path, err := ResolveSecurePath("/app/configs", "manifests/deployment.yaml")
//	// path = "/app/configs/manifests/deployment.yaml"
//
//	path, err := ResolveSecurePath("/app/configs", "../../../etc/passwd")
//	// err = "path \"../../../etc/passwd\" escapes base directory"
func ResolveSecurePath(baseDir, refPath string) (string, error) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}
	baseClean := filepath.Clean(baseAbs)

	var targetPath string
	if filepath.IsAbs(refPath) {
		targetPath = filepath.Clean(refPath)
	} else {
		targetPath = filepath.Clean(filepath.Join(baseClean, refPath))
	}

	// Check if target path is within base directory
	rel, err := filepath.Rel(baseClean, targetPath)
	if err != nil {
		return "", fmt.Errorf("path %q escapes base directory", refPath)
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q escapes base directory", refPath)
	}

	return targetPath, nil
}
