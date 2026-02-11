package manifest

import (
	"encoding/json"
	"fmt"
	"os"

	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/yaml"
)

// ParseManifestWork parses raw bytes (JSON or YAML) into a ManifestWork.
// It first tries JSON; if that fails, it converts from YAML to JSON then parses.
func ParseManifestWork(data []byte) (*workv1.ManifestWork, error) {
	work := &workv1.ManifestWork{}

	// Try JSON first
	if err := json.Unmarshal(data, work); err == nil {
		return work, nil
	}

	// Fall back to YAML → JSON → ManifestWork
	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to convert ManifestWork YAML to JSON: %w", err)
	}

	if err := json.Unmarshal(jsonData, work); err != nil {
		return nil, fmt.Errorf("failed to parse ManifestWork: %w", err)
	}

	return work, nil
}

// LoadManifestWork reads a ManifestWork from a file path (JSON or YAML).
func LoadManifestWork(path string) (*workv1.ManifestWork, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read ManifestWork file %s: %w", path, err)
	}

	work, err := ParseManifestWork(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ManifestWork from %s: %w", path, err)
	}

	return work, nil
}
