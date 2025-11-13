package k8sclient

import (
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceTracker_TrackResource(t *testing.T) {
	mockLog := &mockLogger{}
	
	// Create a mock client (we don't need a real one for this test)
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	// Create a test resource
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test-namespace",
			},
		},
	}
	resource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	})

	// Track the resource
	tracker.TrackResource("testNamespace", resource)

	// Verify it was tracked
	tracked, exists := tracker.GetTrackedResource("testNamespace")
	if !exists {
		t.Error("Resource was not tracked")
	}
	if tracked.Alias != "testNamespace" {
		t.Errorf("Expected alias testNamespace, got %s", tracked.Alias)
	}
	if tracked.Name != "test-namespace" {
		t.Errorf("Expected name test-namespace, got %s", tracked.Name)
	}
	if tracked.GVK.Kind != "Namespace" {
		t.Errorf("Expected kind Namespace, got %s", tracked.GVK.Kind)
	}
}

func TestResourceTracker_GetTrackedResource(t *testing.T) {
	mockLog := &mockLogger{}
	
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	// Test getting non-existent resource
	_, exists := tracker.GetTrackedResource("nonexistent")
	if exists {
		t.Error("Expected resource to not exist")
	}

	// Add a resource
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "test-pod",
				"namespace": "default",
			},
		},
	}
	resource.SetGroupVersionKind(CommonResourceKinds.Pod)
	
	tracker.TrackResource("testPod", resource)

	// Test getting existing resource
	tracked, exists := tracker.GetTrackedResource("testPod")
	if !exists {
		t.Error("Expected resource to exist")
	}
	if tracked.Name != "test-pod" {
		t.Errorf("Expected name test-pod, got %s", tracked.Name)
	}
	if tracked.Namespace != "default" {
		t.Errorf("Expected namespace default, got %s", tracked.Namespace)
	}
}

func TestResourceTracker_GetAllTrackedResources(t *testing.T) {
	mockLog := &mockLogger{}
	
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	// Initially empty
	all := tracker.GetAllTrackedResources()
	if len(all) != 0 {
		t.Errorf("Expected 0 resources, got %d", len(all))
	}

	// Add multiple resources
	for i, name := range []string{"ns1", "ns2", "ns3"} {
		resource := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": name,
				},
			},
		}
		resource.SetGroupVersionKind(CommonResourceKinds.Namespace)
		tracker.TrackResource(name, resource)
		
		all = tracker.GetAllTrackedResources()
		if len(all) != i+1 {
			t.Errorf("Expected %d resources, got %d", i+1, len(all))
		}
	}
}

func TestResourceTracker_ExtractStatus(t *testing.T) {
	mockLog := &mockLogger{}
	
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	tests := []struct {
		name       string
		resource   *unstructured.Unstructured
		alias      string
		wantErr    bool
		wantStatus map[string]interface{}
	}{
		{
			name: "resource with status",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
					},
					"status": map[string]interface{}{
						"phase": "Running",
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "Ready",
								"status": "True",
							},
						},
					},
				},
			},
			alias:   "testPod",
			wantErr: false,
			wantStatus: map[string]interface{}{
				"phase":      "Running",
				"conditions": []interface{}{},
			},
		},
		{
			name: "resource without status",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test-config",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			alias:      "testConfig",
			wantErr:    false,
			wantStatus: map[string]interface{}{},
		},
		{
			name:       "nonexistent resource",
			resource:   nil,
			alias:      "nonexistent",
			wantErr:    true,
			wantStatus: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.resource != nil {
				tracker.TrackResource(tt.alias, tt.resource)
			}

			got, err := tracker.ExtractStatus(tt.alias)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && tt.wantStatus != nil {
				if len(got) != len(tt.wantStatus) {
					t.Errorf("ExtractStatus() returned %d fields, want %d", len(got), len(tt.wantStatus))
				}
				for key := range tt.wantStatus {
					if _, ok := got[key]; !ok {
						t.Errorf("ExtractStatus() missing key %s", key)
					}
				}
			}
		})
	}
}

func TestResourceTracker_ExtractField(t *testing.T) {
	mockLog := &mockLogger{}
	
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "test-pod",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app": "myapp",
				},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "nginx",
						"image": "nginx:latest",
					},
				},
			},
		},
	}
	
	tracker.TrackResource("testPod", resource)

	tests := []struct {
		name      string
		alias     string
		fieldPath []string
		want      interface{}
		wantErr   bool
	}{
		{
			name:      "extract metadata name",
			alias:     "testPod",
			fieldPath: []string{"metadata", "name"},
			want:      "test-pod",
			wantErr:   false,
		},
		{
			name:      "extract nested field",
			alias:     "testPod",
			fieldPath: []string{"metadata", "labels", "app"},
			want:      "myapp",
			wantErr:   false,
		},
		{
			name:      "nonexistent field",
			alias:     "testPod",
			fieldPath: []string{"status", "phase"},
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "nonexistent resource",
			alias:     "nonexistent",
			fieldPath: []string{"metadata", "name"},
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tracker.ExtractField(tt.alias, tt.fieldPath...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractField() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ExtractField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceTracker_BuildVariablesMap(t *testing.T) {
	mockLog := &mockLogger{}
	
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	// Add multiple resources
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test-namespace",
			},
		},
	}
	tracker.TrackResource("testNamespace", ns)

	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
			},
		},
	}
	tracker.TrackResource("testPod", pod)

	// Build variables map
	vars := tracker.BuildVariablesMap()

	// Check structure
	if vars == nil {
		t.Fatal("BuildVariablesMap() returned nil")
	}
	
	resources, ok := vars["resources"].(map[string]interface{})
	if !ok {
		t.Fatal("BuildVariablesMap() missing 'resources' key")
	}

	if len(resources) != 2 {
		t.Errorf("Expected 2 resources in map, got %d", len(resources))
	}

	if _, ok := resources["testNamespace"]; !ok {
		t.Error("Missing testNamespace in resources map")
	}

	if _, ok := resources["testPod"]; !ok {
		t.Error("Missing testPod in resources map")
	}
}

func TestResourceTracker_Clear(t *testing.T) {
	mockLog := &mockLogger{}
	
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	// Add resources
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test-namespace",
			},
		},
	}
	tracker.TrackResource("test", resource)

	if tracker.Count() != 1 {
		t.Errorf("Expected count 1, got %d", tracker.Count())
	}

	// Clear
	tracker.Clear()

	if tracker.Count() != 0 {
		t.Errorf("Expected count 0 after clear, got %d", tracker.Count())
	}

	_, exists := tracker.GetTrackedResource("test")
	if exists {
		t.Error("Resource should not exist after clear")
	}
}

func TestResourceTracker_Count(t *testing.T) {
	mockLog := &mockLogger{}
	
	tracker := &ResourceTracker{
		client:    nil,
		resources: make(map[string]*TrackedResource),
		log:       mockLog,
	}

	if tracker.Count() != 0 {
		t.Errorf("Expected initial count 0, got %d", tracker.Count())
	}

	// Add resources
	for i := 0; i < 5; i++ {
		resource := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "test-namespace",
				},
			},
		}
		tracker.TrackResource(string(rune('a'+i)), resource)
		
		expected := i + 1
		if tracker.Count() != expected {
			t.Errorf("Expected count %d, got %d", expected, tracker.Count())
		}
	}
}

// Mock logger for testing
type mockLogger struct{}

func (m *mockLogger) V(level int32) logger.Logger { return m }
func (m *mockLogger) Infof(format string, args ...interface{}) {}
func (m *mockLogger) Extra(key string, value interface{}) logger.Logger { return m }
func (m *mockLogger) Info(message string) {}
func (m *mockLogger) Warning(message string) {}
func (m *mockLogger) Error(message string) {}
func (m *mockLogger) Fatal(message string) {}

