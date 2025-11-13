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

// TestIntegration_CreateResourceFromTemplate tests template-based resource creation
func TestIntegration_CreateResourceFromTemplate(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("create namespace from template", func(t *testing.T) {
		timestamp := time.Now().Format("20060102150405")
		template := `
apiVersion: v1
kind: Namespace
metadata:
  name: cluster-{{ .clusterId }}-{{ .timestamp }}
  labels:
    cluster-id: "{{ .clusterId }}"
    managed-by: hyperfleet
    timestamp: "{{ .timestamp }}"
`

		variables := map[string]interface{}{
			"clusterId": "test-123",
			"timestamp": timestamp,
		}

		tmpl := k8sclient.ResourceTemplate{
			Template: template,
		}
		created, err := env.Manager.CreateResourceFromTemplate(env.Ctx, tmpl, variables)
		require.NoError(t, err)
		require.NotNil(t, created)

		// Verify namespace was created with correct name and labels
		expectedName := "cluster-test-123-" + timestamp
		assert.Equal(t, expectedName, created.GetName())

		labels := created.GetLabels()
		assert.Equal(t, "test-123", labels["cluster-id"])
		assert.Equal(t, "hyperfleet", labels["managed-by"])
		assert.Equal(t, timestamp, labels["timestamp"])
	})

	t.Run("create configmap from template with sprig functions", func(t *testing.T) {
		timestamp := time.Now().Format("20060102150405")
		template := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .name | lower }}
  namespace: {{ .namespace | default "default" }}
  labels:
    app: {{ .appName }}
data:
  uppercased: {{ .value | upper }}
  quoted: {{ .value | quote }}
  timestamp: "{{ .timestamp }}"
`

		variables := map[string]interface{}{
			"name":      "TEST-CONFIGMAP-" + timestamp,
			"appName":   "myapp",
			"value":     "hello-world",
			"timestamp": timestamp,
		}

		tmpl := k8sclient.ResourceTemplate{
			Template: template,
		}
		created, err := env.Manager.CreateResourceFromTemplate(env.Ctx, tmpl, variables)
		require.NoError(t, err)
		require.NotNil(t, created)

		// Verify lowercasing worked
		expectedName := "test-configmap-" + timestamp
		assert.Equal(t, expectedName, created.GetName())
		assert.Equal(t, "default", created.GetNamespace())

		// Verify data
		data, found, err := unstructured.NestedStringMap(created.Object, "data")
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "HELLO-WORLD", data["uppercased"])
		// The quote function adds quotes in YAML, but Kubernetes stores the value without them
		assert.Equal(t, "hello-world", data["quoted"])
	})
}

// TestIntegration_CreateOrUpdateResource tests create-or-update logic
func TestIntegration_CreateOrUpdateResource(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	cmName := "test-create-or-update-" + time.Now().Format("20060102150405")

	t.Run("creates new resource when not exists", func(t *testing.T) {
		template := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: ` + cmName + `
  namespace: default
data:
  version: "1"
  status: "created"
`

		tmpl := k8sclient.ResourceTemplate{
			Template: template,
		}
		variables := map[string]interface{}{}

		// Render the template to create the object
		obj, err := k8sclient.RenderAndParseResource(tmpl.Template, variables)
		require.NoError(t, err)

		// Try to create it
		created, err := env.Manager.CreateOrUpdateResource(env.Ctx, obj)
		require.NoError(t, err)
		require.NotNil(t, created)

		data, _, _ := unstructured.NestedString(created.Object, "data", "status")
		assert.Equal(t, "created", data)
	})

	t.Run("updates existing resource", func(t *testing.T) {
		template := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: ` + cmName + `
  namespace: default
data:
  version: "2"
  status: "updated"
`

		tmpl := k8sclient.ResourceTemplate{
			Template: template,
		}
		variables := map[string]interface{}{}

		// Render the template to create the object
		obj, err := k8sclient.RenderAndParseResource(tmpl.Template, variables)
		require.NoError(t, err)

		// This should update the existing resource
		updated, err := env.Manager.CreateOrUpdateResource(env.Ctx, obj)
		require.NoError(t, err)
		require.NotNil(t, updated)

		data, _, _ := unstructured.NestedStringMap(updated.Object, "data")
		assert.Equal(t, "2", data["version"])
		assert.Equal(t, "updated", data["status"])
	})
}

// TestIntegration_ResourceExists tests resource existence checking
func TestIntegration_ResourceExists(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")
	cmName := "test-exists-" + timestamp

	// Create a configmap first
	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      cmName,
				"namespace": "default",
				"labels": map[string]interface{}{
					"test-group": timestamp,
					"app":        "myapp",
				},
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}
	cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)
	_, err := env.Client.CreateResource(env.Ctx, cm)
	require.NoError(t, err)

	t.Run("check existing resource by name", func(t *testing.T) {
		discovery := k8sclient.DiscoveryConfig{
			Namespace: "default",
			ByName: &k8sclient.DiscoveryByName{
				Name: cmName,
			},
		}

		exists, found, err := env.Manager.ResourceExists(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, discovery, nil)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.NotNil(t, found)
		assert.Equal(t, cmName, found.GetName())
	})

	t.Run("check existing resource by label selector", func(t *testing.T) {
		discovery := k8sclient.DiscoveryConfig{
			Namespace: "default",
			BySelectors: &k8sclient.DiscoveryBySelectors{
				LabelSelector: map[string]string{
					"test-group": timestamp,
					"app":        "myapp",
				},
			},
		}

		exists, found, err := env.Manager.ResourceExists(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, discovery, nil)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.NotNil(t, found)
	})

	t.Run("check non-existent resource", func(t *testing.T) {
		discovery := k8sclient.DiscoveryConfig{
			Namespace: "default",
			ByName: &k8sclient.DiscoveryByName{
				Name: "non-existent-configmap-12345",
			},
		}

		exists, found, err := env.Manager.ResourceExists(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, discovery, nil)
		require.NoError(t, err)
		assert.False(t, exists)
		assert.Nil(t, found)
	})
}

// TestIntegration_DiscoverAndTrack tests resource discovery and tracking
func TestIntegration_DiscoverAndTrack(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")
	nsName := "test-track-ns-" + timestamp

	// Create namespace first
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": nsName,
				"labels": map[string]interface{}{
					"test-group": timestamp,
				},
			},
		},
	}
	ns.SetGroupVersionKind(k8sclient.CommonResourceKinds.Namespace)
	_, err := env.Client.CreateResource(env.Ctx, ns)
	require.NoError(t, err)

	t.Run("discover and track by name", func(t *testing.T) {
		trackConfig := k8sclient.TrackConfig{
			As: "myNamespace",
			Discovery: k8sclient.DiscoveryConfig{
				Namespace: "", // Namespace is cluster-scoped
				ByName: &k8sclient.DiscoveryByName{
					Name: nsName,
				},
			},
		}

		err := env.Manager.DiscoverAndTrack(env.Ctx, k8sclient.CommonResourceKinds.Namespace, trackConfig, nil)
		require.NoError(t, err)

		// Verify it's tracked
		tracked, exists := env.Manager.GetTracker().GetTrackedResource("myNamespace")
		assert.True(t, exists)
		assert.NotNil(t, tracked)
		assert.Equal(t, nsName, tracked.Name)
	})

	t.Run("discover with templated namespace", func(t *testing.T) {
		// Create a configmap in default namespace
		cmName := "test-templated-" + timestamp
		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      cmName,
					"namespace": "default",
				},
			},
		}
		cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)
		_, err := env.Client.CreateResource(env.Ctx, cm)
		require.NoError(t, err)

		// Track with templated namespace
		trackConfig := k8sclient.TrackConfig{
			As: "myConfigMap",
			Discovery: k8sclient.DiscoveryConfig{
				Namespace: "{{ .targetNamespace }}",
				ByName: &k8sclient.DiscoveryByName{
					Name: cmName,
				},
			},
		}

		variables := map[string]interface{}{
			"targetNamespace": "default",
		}

		err = env.Manager.DiscoverAndTrack(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, trackConfig, variables)
		require.NoError(t, err)

		// Verify tracking
		tracked, exists := env.Manager.GetTracker().GetTrackedResource("myConfigMap")
		assert.True(t, exists)
		assert.Equal(t, cmName, tracked.Name)
		assert.Equal(t, "default", tracked.Namespace)
	})
}

// TestIntegration_GetTrackedResourcesAsVariables tests variables extraction
func TestIntegration_GetTrackedResourcesAsVariables(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	timestamp := time.Now().Format("20060102150405")

	// Create and track a namespace
	nsName := "test-vars-ns-" + timestamp
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

	env.Manager.GetTracker().TrackResource("testNamespace", created)

	// Get variables
	vars := env.Manager.GetTrackedResourcesAsVariables()
	require.NotNil(t, vars)

	// Verify structure
	resources, ok := vars["resources"].(map[string]interface{})
	require.True(t, ok)

	testNs, ok := resources["testNamespace"]
	require.True(t, ok)
	require.NotNil(t, testNs)

	// Verify can access fields
	nsMap, ok := testNs.(map[string]interface{})
	require.True(t, ok)

	metadata, ok := nsMap["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, nsName, metadata["name"])
}
