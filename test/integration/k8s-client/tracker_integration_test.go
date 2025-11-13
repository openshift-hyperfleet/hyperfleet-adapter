// +build integration

package k8sclient_integration

import (
	"testing"
	"time"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s-client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestIntegration_DiscoverAndTrackResource tests resource discovery
func TestIntegration_DiscoverAndTrackResource(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")

	t.Run("discover namespace by name", func(t *testing.T) {
		nsName := "test-discover-ns-" + timestamp

		// Create test namespace
		ns := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": nsName,
					"labels": map[string]interface{}{
						"test": "discovery",
					},
				},
			},
		}
		ns.SetGroupVersionKind(k8sclient.CommonResourceKinds.Namespace)
		_, err := env.Client.CreateResource(env.Ctx, ns)
		require.NoError(t, err)

		// Discover by name (tracker expects already-rendered values)
		err = env.Manager.GetTracker().DiscoverAndTrackByName(env.Ctx, "discoveredNamespace", k8sclient.CommonResourceKinds.Namespace, "", nsName)
		require.NoError(t, err)

		// Verify it's tracked correctly
		tracked, exists := env.Manager.GetTracker().GetTrackedResource("discoveredNamespace")
		require.True(t, exists)
		assert.Equal(t, nsName, tracked.Name)
		assert.Equal(t, "Namespace", tracked.GVK.Kind)
	})

	t.Run("discover configmaps by label selector", func(t *testing.T) {
		testLabel := "discovery-test-" + timestamp

		// Create multiple configmaps with labels
		for i := 1; i <= 2; i++ {
			cm := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm-" + timestamp + "-" + string(rune('a'+i)),
						"namespace": "default",
						"labels": map[string]interface{}{
							"test-group": testLabel,
							"index":      string(rune('0' + i)),
						},
					},
					"data": map[string]interface{}{
						"index": string(rune('0' + i)),
					},
				},
			}
			cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)
			_, err := env.Client.CreateResource(env.Ctx, cm)
			require.NoError(t, err)
		}

		// Discover by label selector (tracker expects already-rendered values)
		labelSelector := "test-group=" + testLabel + ",index=1"
		err := env.Manager.GetTracker().DiscoverAndTrackBySelectors(env.Ctx, "discoveredConfigMap", k8sclient.CommonResourceKinds.ConfigMap, "default", labelSelector)
		require.NoError(t, err)

		// Verify correct pod is found
		tracked, exists := env.Manager.GetTracker().GetTrackedResource("discoveredConfigMap")
		require.True(t, exists)
		assert.Equal(t, "default", tracked.Namespace)
	})

	t.Run("discover with templated values", func(t *testing.T) {
		cmName := "test-templated-cm-" + timestamp

		// Create configmap
		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      cmName,
					"namespace": "default",
					"labels": map[string]interface{}{
						"cluster-id": "test-123",
						"app":        "myapp",
					},
				},
			},
		}
		cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)
		_, err := env.Client.CreateResource(env.Ctx, cm)
		require.NoError(t, err)

		// Discover with already-rendered values
		// (In real usage, manager renders templates before calling tracker)
		labelSelector := "cluster-id=test-123,app=myapp"
		err = env.Manager.GetTracker().DiscoverAndTrackBySelectors(env.Ctx, "templatedResource", k8sclient.CommonResourceKinds.ConfigMap, "default", labelSelector)
		require.NoError(t, err)

		tracked, exists := env.Manager.GetTracker().GetTrackedResource("templatedResource")
		require.True(t, exists)
		assert.Equal(t, "default", tracked.Namespace)
	})
}

// TestIntegration_RefreshResource tests refreshing tracked resource from cluster
func TestIntegration_RefreshResource(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")
	cmName := "test-refresh-cm-" + timestamp

	t.Run("refresh configmap after modification", func(t *testing.T) {
		// Create and track configmap
		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      cmName,
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"version": "1",
				},
			},
		}
		cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)
		created, err := env.Client.CreateResource(env.Ctx, cm)
		require.NoError(t, err)

		env.Manager.GetTracker().TrackResource("refreshTest", created)

		// Modify it directly in cluster
		err = unstructured.SetNestedField(created.Object, "2", "data", "version")
		require.NoError(t, err)
		_, err = env.Client.UpdateResource(env.Ctx, created)
		require.NoError(t, err)

		// Refresh tracking
		err = env.Manager.GetTracker().RefreshResource(env.Ctx, "refreshTest")
		require.NoError(t, err)

		// Verify local copy is updated
		tracked, exists := env.Manager.GetTracker().GetTrackedResource("refreshTest")
		require.True(t, exists)

		data, _, _ := unstructured.NestedString(tracked.Resource.Object, "data", "version")
		assert.Equal(t, "2", data, "Tracked resource should have updated value")
	})
}

// TestIntegration_RefreshAllResources tests refreshing all tracked resources
func TestIntegration_RefreshAllResources(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")

	t.Run("refresh multiple resources", func(t *testing.T) {
		// Create and track multiple configmaps
		for i := 1; i <= 3; i++ {
			cmName := "test-refresh-all-" + timestamp + "-" + string(rune('a'+i))
			cm := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      cmName,
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"version": "1",
						"index":   string(rune('0' + i)),
					},
				},
			}
			cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)
			created, err := env.Client.CreateResource(env.Ctx, cm)
			require.NoError(t, err)

			alias := "cm" + string(rune('0'+i))
			env.Manager.GetTracker().TrackResource(alias, created)

			// Modify in cluster
			err = unstructured.SetNestedField(created.Object, "2", "data", "version")
			require.NoError(t, err)
			_, err = env.Client.UpdateResource(env.Ctx, created)
			require.NoError(t, err)
		}

		// Refresh all resources
		err := env.Manager.GetTracker().RefreshAllResources(env.Ctx)
		require.NoError(t, err)

		// Verify all are updated
		for i := 1; i <= 3; i++ {
			alias := "cm" + string(rune('0'+i))
			tracked, exists := env.Manager.GetTracker().GetTrackedResource(alias)
			require.True(t, exists)

			data, _, _ := unstructured.NestedString(tracked.Resource.Object, "data", "version")
			assert.Equal(t, "2", data, "All tracked resources should be updated")
		}
	})
}

// TestIntegration_StatusExtraction tests extracting status from live resources
func TestIntegration_StatusExtraction(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")

	t.Run("extract namespace status", func(t *testing.T) {
		nsName := "test-status-ns-" + timestamp

		// Create namespace
		ns := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": nsName,
				},
			},
		}
		ns.SetGroupVersionKind(k8sclient.CommonResourceKinds.Namespace)
		created, err := env.Client.CreateResource(env.Ctx, ns)
		require.NoError(t, err)

		// Track it
		env.Manager.GetTracker().TrackResource("statusTest", created)

		// Wait a bit for status to be populated
		time.Sleep(100 * time.Millisecond)

		// Refresh to get latest status
		err = env.Manager.GetTracker().RefreshResource(env.Ctx, "statusTest")
		require.NoError(t, err)

		// Extract status
		status, err := env.Manager.GetTracker().ExtractStatus("statusTest")
		require.NoError(t, err)
		require.NotNil(t, status)

		// Namespace should have a phase in status
		if phase, ok := status["phase"]; ok {
			assert.NotEmpty(t, phase, "Namespace should have a phase")
		}
	})
}

// TestIntegration_VariablesMapWithLiveResources tests building variables from live K8s resources
func TestIntegration_VariablesMapWithLiveResources(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")

	t.Run("build variables from tracked resources", func(t *testing.T) {
		// Create and track a namespace
		nsName := "test-vars-" + timestamp
		ns := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": nsName,
				},
			},
		}
		ns.SetGroupVersionKind(k8sclient.CommonResourceKinds.Namespace)
		createdNs, err := env.Client.CreateResource(env.Ctx, ns)
		require.NoError(t, err)
		env.Manager.GetTracker().TrackResource("myNamespace", createdNs)

		// Create and track a configmap
		cmName := "test-vars-cm-" + timestamp
		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      cmName,
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		}
		cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)
		createdCm, err := env.Client.CreateResource(env.Ctx, cm)
		require.NoError(t, err)
		env.Manager.GetTracker().TrackResource("myConfigMap", createdCm)

		// Build variables map
		vars := env.Manager.GetTracker().BuildVariablesMap()
		require.NotNil(t, vars)

		// Verify structure
		resources, ok := vars["resources"].(map[string]interface{})
		require.True(t, ok)
		assert.Len(t, resources, 2)

		// Verify can access fields
		myNs, ok := resources["myNamespace"]
		require.True(t, ok)
		nsMap := myNs.(map[string]interface{})
		metadata := nsMap["metadata"].(map[string]interface{})
		assert.Equal(t, nsName, metadata["name"])

		myCm, ok := resources["myConfigMap"]
		require.True(t, ok)
		cmMap := myCm.(map[string]interface{})
		data := cmMap["data"].(map[string]interface{})
		assert.Equal(t, "value", data["key"])
	})

	t.Run("use variables in subsequent templates", func(t *testing.T) {
		// Track a namespace
		nsName := "test-template-vars-" + timestamp
		ns := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": nsName,
				},
			},
		}
		ns.SetGroupVersionKind(k8sclient.CommonResourceKinds.Namespace)
		createdNs, err := env.Client.CreateResource(env.Ctx, ns)
		require.NoError(t, err)
		env.Manager.GetTracker().TrackResource("templateNamespace", createdNs)

		// Use tracked namespace name in configmap template
		cmTemplate := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: derived-from-ns
  namespace: default
data:
  sourceNamespace: "{{ .resources.templateNamespace.metadata.name }}"
  timestamp: "` + timestamp + `"
`

		vars := env.Manager.GetTracker().BuildVariablesMap()
		tmpl := k8sclient.ResourceTemplate{
			Template: cmTemplate,
		}
		created, err := env.Manager.CreateResourceFromTemplate(env.Ctx, tmpl, vars)
		require.NoError(t, err)

		// Verify the template used tracked resource data
		data, _, _ := unstructured.NestedString(created.Object, "data", "sourceNamespace")
		assert.Equal(t, nsName, data, "Template should use tracked namespace name")
	})
}
