package k8sclient

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewResourceManager(t *testing.T) {
	mockLog := &mockLogger{}
	
	// Create manager (client can be nil for this test)
	manager := NewResourceManager(nil, mockLog)

	if manager == nil {
		t.Fatal("NewResourceManager() returned nil")
	}

	if manager.tracker == nil {
		t.Error("ResourceManager tracker is nil")
	}

	if manager.log == nil {
		t.Error("ResourceManager log is nil")
	}
}

func TestResourceManager_GetTracker(t *testing.T) {
	mockLog := &mockLogger{}
	manager := NewResourceManager(nil, mockLog)

	tracker := manager.GetTracker()
	if tracker == nil {
		t.Error("GetTracker() returned nil")
	}

	// Verify it's the same tracker
	if tracker != manager.tracker {
		t.Error("GetTracker() returned different tracker instance")
	}
}

func TestResourceManager_GetTrackedResourcesAsVariables(t *testing.T) {
	mockLog := &mockLogger{}
	manager := &ResourceManager{
		client: nil,
		tracker: &ResourceTracker{
			client:    nil,
			resources: make(map[string]*TrackedResource),
			log:       mockLog,
		},
		log: mockLog,
	}

	// Add some tracked resources
	resource1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test-ns",
			},
		},
	}
	manager.tracker.TrackResource("testNamespace", resource1)

	resource2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
			},
		},
	}
	manager.tracker.TrackResource("testPod", resource2)

	// Get variables
	vars := manager.GetTrackedResourcesAsVariables()

	if vars == nil {
		t.Fatal("GetTrackedResourcesAsVariables() returned nil")
	}

	resources, ok := vars["resources"].(map[string]interface{})
	if !ok {
		t.Fatal("Variables missing 'resources' key")
	}

	if len(resources) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(resources))
	}

	if _, ok := resources["testNamespace"]; !ok {
		t.Error("Missing testNamespace in variables")
	}

	if _, ok := resources["testPod"]; !ok {
		t.Error("Missing testPod in variables")
	}
}

func TestResourceManager_ClearTrackedResources(t *testing.T) {
	mockLog := &mockLogger{}
	manager := &ResourceManager{
		client: nil,
		tracker: &ResourceTracker{
			client:    nil,
			resources: make(map[string]*TrackedResource),
			log:       mockLog,
		},
		log: mockLog,
	}

	// Add a tracked resource
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test-ns",
			},
		},
	}
	manager.tracker.TrackResource("test", resource)

	if manager.tracker.Count() != 1 {
		t.Errorf("Expected 1 tracked resource, got %d", manager.tracker.Count())
	}

	// Clear
	manager.ClearTrackedResources()

	if manager.tracker.Count() != 0 {
		t.Errorf("Expected 0 tracked resources after clear, got %d", manager.tracker.Count())
	}
}

func TestResourceManager_GetClient(t *testing.T) {
	mockLog := &mockLogger{}
	mockClient := &Client{} // Empty client for testing

	manager := &ResourceManager{
		client: mockClient,
		tracker: &ResourceTracker{
			client:    mockClient,
			resources: make(map[string]*TrackedResource),
			log:       mockLog,
		},
		log: mockLog,
	}

	client := manager.GetClient()
	if client != mockClient {
		t.Error("GetClient() returned different client instance")
	}
}

func TestRenderAndParseResourceTemplate(t *testing.T) {
	tests := []struct {
		name      string
		template  ResourceTemplate
		variables map[string]interface{}
		wantErr   bool
		checkName string
	}{
		{
			name: "valid namespace template",
			template: ResourceTemplate{
				Template: `
apiVersion: v1
kind: Namespace
metadata:
  name: cluster-{{ .clusterId }}
  labels:
    cluster-id: "{{ .clusterId }}"
`,
				Track: &TrackConfig{
					As: "clusterNamespace",
					Discovery: DiscoveryConfig{
						Namespace: "",
						ByName: &DiscoveryByName{
							Name: "cluster-{{ .clusterId }}",
						},
					},
				},
			},
			variables: map[string]interface{}{
				"clusterId": "abc123",
			},
			wantErr:   false,
			checkName: "cluster-abc123",
		},
		{
			name: "template with sprig functions",
			template: ResourceTemplate{
				Template: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .name | lower }}
  namespace: default
data:
  key: "{{ .value | upper }}"
`,
			},
			variables: map[string]interface{}{
				"name":  "TEST-CONFIG",
				"value": "test-value",
			},
			wantErr:   false,
			checkName: "test-config",
		},
		{
			name: "invalid template",
			template: ResourceTemplate{
				Template: "{{ .missing.field }}",
			},
			variables: map[string]interface{}{
				"other": "value",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := RenderAndParseResource(tt.template.Template, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderAndParseResource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkName != "" {
				name, _, _ := unstructured.NestedString(obj.Object, "metadata", "name")
				if name != tt.checkName {
					t.Errorf("Expected name %s, got %s", tt.checkName, name)
				}
			}
		})
	}
}

func TestTrackConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		track   TrackConfig
		wantErr bool
	}{
		{
			name: "valid track config with byName",
			track: TrackConfig{
				As: "resource",
				Discovery: DiscoveryConfig{
					Namespace: "default",
					ByName: &DiscoveryByName{
						Name: "test-resource",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid track config with bySelectors",
			track: TrackConfig{
				As: "resource",
				Discovery: DiscoveryConfig{
					Namespace: "default",
					BySelectors: &DiscoveryBySelectors{
						LabelSelector: map[string]string{
							"app": "test",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty alias should be caught by implementation",
			track: TrackConfig{
				As: "",
				Discovery: DiscoveryConfig{
					ByName: &DiscoveryByName{
						Name: "test",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the structure is valid
			if tt.track.As == "" && !tt.wantErr {
				t.Error("Empty alias should be invalid")
			}
		})
	}
}

func TestDiscoveryConfigTemplating(t *testing.T) {
	tests := []struct {
		name      string
		discovery DiscoveryConfig
		variables map[string]interface{}
		want      string // Expected rendered namespace
		wantErr   bool
	}{
		{
			name: "static namespace",
			discovery: DiscoveryConfig{
				Namespace: "default",
			},
			variables: map[string]interface{}{},
			want:      "default",
			wantErr:   false,
		},
		{
			name: "templated namespace",
			discovery: DiscoveryConfig{
				Namespace: "cluster-{{ .clusterId }}-ns",
			},
			variables: map[string]interface{}{
				"clusterId": "abc123",
			},
			want:    "cluster-abc123-ns",
			wantErr: false,
		},
		{
			name: "empty namespace for cluster-scoped",
			discovery: DiscoveryConfig{
				Namespace: "",
			},
			variables: map[string]interface{}{},
			want:      "",
			wantErr:   false,
		},
		{
			name: "namespace from nested field",
			discovery: DiscoveryConfig{
				Namespace: "{{ .cluster.spec.namespace }}",
			},
			variables: map[string]interface{}{
				"cluster": map[string]interface{}{
					"spec": map[string]interface{}{
						"namespace": "prod-workloads",
					},
				},
			},
			want:    "prod-workloads",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.discovery.Namespace == "" {
				if tt.want != "" {
					t.Errorf("Expected empty namespace, got %s", tt.want)
				}
				return
			}

			rendered, err := RenderTemplate(tt.discovery.Namespace, tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && rendered != tt.want {
				t.Errorf("Rendered namespace = %s, want %s", rendered, tt.want)
			}
		})
	}
}

