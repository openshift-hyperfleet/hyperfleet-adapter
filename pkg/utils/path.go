package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveSecurePath resolves a path against the base directory.
// - Absolute paths are returned as-is
// - Relative paths are resolved against the base directory and validated to not escape it
func ResolveSecurePath(baseDir, refPath string) (string, error) {
	if filepath.IsAbs(refPath) {
		return filepath.Clean(refPath), nil
	}

	fullPath := filepath.Join(baseDir, refPath)
	fullPath = filepath.Clean(fullPath)

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if !strings.HasPrefix(absPath, absBase) {
		return "", fmt.Errorf("path %q escapes base directory %q", refPath, baseDir)
	}

	return fullPath, nil
}
