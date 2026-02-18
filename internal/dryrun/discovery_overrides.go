package dryrun

import (
	"encoding/json"
	"fmt"
	"os"
)

// DiscoveryOverrides maps rendered Kubernetes resource names to complete resource
// objects that replace applied manifests in the in-memory store. This allows
// dry-run mode to simulate server-populated fields (status, uid, resourceVersion, etc.).
type DiscoveryOverrides map[string]map[string]interface{}

// LoadDiscoveryOverrides reads a JSON file and returns discovery overrides.
// Each top-level key is a rendered metadata.name, and each value is a complete
// Kubernetes-like resource object that must contain at least apiVersion and kind.
func LoadDiscoveryOverrides(path string) (DiscoveryOverrides, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery overrides file: %w", err)
	}

	var overrides DiscoveryOverrides
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("failed to parse discovery overrides JSON: %w", err)
	}

	// Validate each entry has required fields
	for name, obj := range overrides {
		if _, ok := obj["apiVersion"]; !ok {
			return nil, fmt.Errorf("discovery override %q is missing required field \"apiVersion\"", name)
		}
		if _, ok := obj["kind"]; !ok {
			return nil, fmt.Errorf("discovery override %q is missing required field \"kind\"", name)
		}
	}

	return overrides, nil
}
