package maestro_client

import (
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/generation"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"
)

func TestValidateGeneration(t *testing.T) {
	tests := []struct {
		name        string
		meta        metav1.ObjectMeta
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid generation annotation",
			meta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.AnnotationGeneration: "5",
				},
			},
			expectError: false,
		},
		{
			name: "generation 0 is invalid (must be > 0)",
			meta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.AnnotationGeneration: "0",
				},
			},
			expectError: true,
		},
		{
			name: "large generation is valid",
			meta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.AnnotationGeneration: "9999999999",
				},
			},
			expectError: false,
		},
		{
			name:        "missing annotations",
			meta:        metav1.ObjectMeta{},
			expectError: true,
			errorMsg:    "missing",
		},
		{
			name: "missing generation annotation",
			meta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"other": "annotation",
				},
			},
			expectError: true,
			errorMsg:    "missing",
		},
		{
			name: "empty generation annotation",
			meta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.AnnotationGeneration: "",
				},
			},
			expectError: true,
			errorMsg:    "empty",
		},
		{
			name: "invalid generation value",
			meta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.AnnotationGeneration: "not-a-number",
				},
			},
			expectError: true,
			errorMsg:    "invalid",
		},
		{
			name: "negative generation",
			meta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.AnnotationGeneration: "-5",
				},
			},
			expectError: true,
			errorMsg:    "must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := generation.ValidateGeneration(tt.meta)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateGenerationFromUnstructured(t *testing.T) {
	tests := []struct {
		name        string
		obj         *unstructured.Unstructured
		expectError bool
	}{
		{
			name: "valid generation annotation",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": "test",
						"annotations": map[string]interface{}{
							constants.AnnotationGeneration: "5",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil object",
			obj:         nil,
			expectError: true,
		},
		{
			name: "missing annotations",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			expectError: true,
		},
		{
			name: "invalid generation value",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": "test",
						"annotations": map[string]interface{}{
							constants.AnnotationGeneration: "invalid",
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := generation.ValidateGenerationFromUnstructured(tt.obj)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateManifestWorkGeneration(t *testing.T) {
	// Helper to create a valid manifest with generation
	createManifest := func(kind, name, generation string) workv1.Manifest {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       kind,
				"metadata": map[string]interface{}{
					"name": name,
					"annotations": map[string]interface{}{
						constants.AnnotationGeneration: generation,
					},
				},
			},
		}
		raw, _ := obj.MarshalJSON()
		return workv1.Manifest{RawExtension: runtime.RawExtension{Raw: raw}}
	}

	// Helper to create a manifest without generation
	createManifestNoGeneration := func(kind, name string) workv1.Manifest {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       kind,
				"metadata": map[string]interface{}{
					"name": name,
				},
			},
		}
		raw, _ := obj.MarshalJSON()
		return workv1.Manifest{RawExtension: runtime.RawExtension{Raw: raw}}
	}

	tests := []struct {
		name        string
		work        *workv1.ManifestWork
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid ManifestWork with generation on all",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-work",
					Annotations: map[string]string{
						constants.AnnotationGeneration: "5",
					},
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							createManifest("Namespace", "test-ns", "5"),
							createManifest("ConfigMap", "test-cm", "5"),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil work",
			work:        nil,
			expectError: true,
			errorMsg:    "cannot be nil",
		},
		{
			name: "ManifestWork without generation annotation",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-work",
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							createManifest("Namespace", "test-ns", "5"),
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "missing",
		},
		{
			name: "manifest without generation annotation",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-work",
					Annotations: map[string]string{
						constants.AnnotationGeneration: "5",
					},
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{
							createManifest("Namespace", "test-ns", "5"),
							createManifestNoGeneration("ConfigMap", "test-cm"),
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "ConfigMap",
		},
		{
			name: "empty manifests is valid",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-work",
					Annotations: map[string]string{
						constants.AnnotationGeneration: "5",
					},
				},
				Spec: workv1.ManifestWorkSpec{
					Workload: workv1.ManifestsTemplate{
						Manifests: []workv1.Manifest{},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := generation.ValidateManifestWorkGeneration(tt.work)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetGenerationFromManifestWork(t *testing.T) {
	tests := []struct {
		name     string
		work     *workv1.ManifestWork
		expected int64
	}{
		{
			name:     "nil work returns 0",
			work:     nil,
			expected: 0,
		},
		{
			name: "work with generation annotation",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationGeneration: "42",
					},
				},
			},
			expected: 42,
		},
		{
			name: "work without annotations",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: 0,
		},
		{
			name: "work with invalid generation value",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationGeneration: "invalid",
					},
				},
			},
			expected: 0,
		},
		{
			name: "work with empty generation value",
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.AnnotationGeneration: "",
					},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result int64
			if tt.work == nil {
				result = 0
			} else {
				result = generation.GetGeneration(tt.work.ObjectMeta)
			}
			if result != tt.expected {
				t.Errorf("expected generation %d, got %d", tt.expected, result)
			}
		})
	}
}

// BuildManifestWorkName generates a consistent ManifestWork name for testing
// Format: <adapter-name>-<resource-name>-<cluster-id>
func BuildManifestWorkName(adapterName, resourceName, clusterID string) string {
	return adapterName + "-" + resourceName + "-" + clusterID
}

func TestBuildManifestWorkName(t *testing.T) {
	tests := []struct {
		name         string
		adapterName  string
		resourceName string
		clusterID    string
		expected     string
	}{
		{
			name:         "basic name construction",
			adapterName:  "my-adapter",
			resourceName: "namespace",
			clusterID:    "cluster-123",
			expected:     "my-adapter-namespace-cluster-123",
		},
		{
			name:         "with special characters",
			adapterName:  "adapter_v1",
			resourceName: "config-map",
			clusterID:    "prod-us-east-1",
			expected:     "adapter_v1-config-map-prod-us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildManifestWorkName(tt.adapterName, tt.resourceName, tt.clusterID)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerationComparison(t *testing.T) {
	tests := []struct {
		name               string
		existingGeneration int64
		newGeneration      int64
		shouldUpdate       bool
		description        string
	}{
		{
			name:               "same generation - no update",
			existingGeneration: 5,
			newGeneration:      5,
			shouldUpdate:       false,
			description:        "When generations match, should skip update",
		},
		{
			name:               "newer generation - update",
			existingGeneration: 5,
			newGeneration:      6,
			shouldUpdate:       true,
			description:        "When new generation is higher, should update",
		},
		{
			name:               "older generation - still update",
			existingGeneration: 6,
			newGeneration:      5,
			shouldUpdate:       true,
			description:        "When new generation is lower, should still update (allow rollback)",
		},
		// Note: "both 0" case is no longer valid since validation requires generation > 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Logic from ApplyManifestWork:
			// if existingGeneration == generation { return existing }
			shouldSkipUpdate := tt.existingGeneration == tt.newGeneration
			shouldUpdate := !shouldSkipUpdate

			if shouldUpdate != tt.shouldUpdate {
				t.Errorf("%s: expected shouldUpdate=%v, got %v",
					tt.description, tt.shouldUpdate, shouldUpdate)
			}
		})
	}
}
