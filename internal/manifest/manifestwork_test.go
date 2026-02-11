package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestWork_JSON(t *testing.T) {
	jsonData := []byte(`{
		"apiVersion": "work.open-cluster-management.io/v1",
		"kind": "ManifestWork",
		"metadata": {
			"name": "test-work",
			"namespace": "cluster1",
			"labels": {
				"app": "test"
			},
			"annotations": {
				"hyperfleet.io/generation": "1"
			}
		},
		"spec": {
			"workload": {
				"manifests": [
					{
						"apiVersion": "v1",
						"kind": "ConfigMap",
						"metadata": {
							"name": "test-cm",
							"namespace": "default"
						}
					}
				]
			}
		}
	}`)

	work, err := ParseManifestWork(jsonData)
	if err != nil {
		t.Fatalf("ParseManifestWork failed for JSON: %v", err)
	}

	if work.Name != "test-work" {
		t.Errorf("expected name 'test-work', got %q", work.Name)
	}
	if work.Namespace != "cluster1" {
		t.Errorf("expected namespace 'cluster1', got %q", work.Namespace)
	}
	if work.Labels["app"] != "test" {
		t.Errorf("expected label app=test, got %v", work.Labels)
	}
	if work.Annotations["hyperfleet.io/generation"] != "1" {
		t.Errorf("expected generation annotation '1', got %v", work.Annotations)
	}
	if len(work.Spec.Workload.Manifests) != 1 {
		t.Errorf("expected 1 manifest, got %d", len(work.Spec.Workload.Manifests))
	}
}

func TestParseManifestWork_YAML(t *testing.T) {
	yamlData := []byte(`apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: test-work-yaml
  namespace: cluster2
  labels:
    env: staging
  annotations:
    hyperfleet.io/generation: "2"
spec:
  workload:
    manifests:
      - apiVersion: v1
        kind: ConfigMap
        metadata:
          name: yaml-cm
          namespace: default
        data:
          key1: value1
`)

	work, err := ParseManifestWork(yamlData)
	if err != nil {
		t.Fatalf("ParseManifestWork failed for YAML: %v", err)
	}

	if work.Name != "test-work-yaml" {
		t.Errorf("expected name 'test-work-yaml', got %q", work.Name)
	}
	if work.Namespace != "cluster2" {
		t.Errorf("expected namespace 'cluster2', got %q", work.Namespace)
	}
	if work.Labels["env"] != "staging" {
		t.Errorf("expected label env=staging, got %v", work.Labels)
	}
	if work.Annotations["hyperfleet.io/generation"] != "2" {
		t.Errorf("expected generation annotation '2', got %v", work.Annotations)
	}
	if len(work.Spec.Workload.Manifests) != 1 {
		t.Errorf("expected 1 manifest, got %d", len(work.Spec.Workload.Manifests))
	}
}

func TestParseManifestWork_InvalidData(t *testing.T) {
	_, err := ParseManifestWork([]byte("not valid json or yaml {{{"))
	if err == nil {
		t.Error("expected error for invalid data, got nil")
	}
}

func TestParseManifestWork_EmptyData(t *testing.T) {
	work, err := ParseManifestWork([]byte("{}"))
	if err != nil {
		t.Fatalf("ParseManifestWork failed for empty JSON: %v", err)
	}
	if work.Name != "" {
		t.Errorf("expected empty name, got %q", work.Name)
	}
}

func TestLoadManifestWork(t *testing.T) {
	// Write a temporary YAML file
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "work.yaml")
	yamlData := []byte(`apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: file-loaded-work
  annotations:
    hyperfleet.io/generation: "3"
spec:
  workload:
    manifests: []
`)
	if err := os.WriteFile(yamlPath, yamlData, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	work, err := LoadManifestWork(yamlPath)
	if err != nil {
		t.Fatalf("LoadManifestWork failed: %v", err)
	}

	if work.Name != "file-loaded-work" {
		t.Errorf("expected name 'file-loaded-work', got %q", work.Name)
	}
	if work.Annotations["hyperfleet.io/generation"] != "3" {
		t.Errorf("expected generation annotation '3', got %v", work.Annotations)
	}
}

func TestLoadManifestWork_FileNotFound(t *testing.T) {
	_, err := LoadManifestWork("/nonexistent/path/work.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
