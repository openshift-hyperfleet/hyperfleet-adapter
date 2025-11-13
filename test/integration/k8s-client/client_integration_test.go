// +build integration

package k8sclient_integration

import (
	"testing"
	"time"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s-client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestIntegration_NewClient tests client initialization with real K8s API
func TestIntegration_NewClient(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("client is properly initialized", func(t *testing.T) {
		assert.NotNil(t, env.Client)
		assert.NotNil(t, env.Config)
	})
}

// TestIntegration_CreateResource tests creating resources in K8s
func TestIntegration_CreateResource(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("create namespace", func(t *testing.T) {
		// Create namespace resource
		ns := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "test-namespace-" + time.Now().Format("20060102150405"),
					"labels": map[string]interface{}{
						"test": "integration",
						"app":  "k8s-client",
					},
				},
			},
		}
		ns.SetGroupVersionKind(k8sclient.CommonResourceKinds.Namespace)

		// Create the namespace
		created, err := env.Client.CreateResource(env.Ctx, ns)
		require.NoError(t, err)
		require.NotNil(t, created)

		// Verify the namespace was created
		assert.Equal(t, "Namespace", created.GetKind())
		assert.Equal(t, ns.GetName(), created.GetName())

		// Verify labels
		labels := created.GetLabels()
		assert.Equal(t, "integration", labels["test"])
		assert.Equal(t, "k8s-client", labels["app"])
	})

	t.Run("create configmap", func(t *testing.T) {
		cmName := "test-configmap-" + time.Now().Format("20060102150405")

		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      cmName,
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			},
		}
		cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)

		created, err := env.Client.CreateResource(env.Ctx, cm)
		require.NoError(t, err)
		require.NotNil(t, created)

		assert.Equal(t, "ConfigMap", created.GetKind())
		assert.Equal(t, cmName, created.GetName())
		assert.Equal(t, "default", created.GetNamespace())
	})
}

// TestIntegration_GetResource tests getting resources from K8s
func TestIntegration_GetResource(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("get existing namespace", func(t *testing.T) {
		nsName := "test-get-ns-" + time.Now().Format("20060102150405")

		// Create namespace first
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

		_, err := env.Client.CreateResource(env.Ctx, ns)
		require.NoError(t, err)

		// Get the namespace
		retrieved, err := env.Client.GetResource(env.Ctx, k8sclient.CommonResourceKinds.Namespace, "", nsName)
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.Equal(t, "Namespace", retrieved.GetKind())
		assert.Equal(t, nsName, retrieved.GetName())
	})

	t.Run("get non-existent resource returns error", func(t *testing.T) {
		_, err := env.Client.GetResource(env.Ctx, k8sclient.CommonResourceKinds.Namespace, "", "non-existent-namespace-12345")
		require.Error(t, err)
	})
}

// TestIntegration_ListResources tests listing resources with selectors
func TestIntegration_ListResources(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("list configmaps with label selector", func(t *testing.T) {
		timestamp := time.Now().Format("20060102150405")

		// Create multiple configmaps with labels
		for i := 1; i <= 3; i++ {
			cm := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm-" + timestamp + "-" + string(rune('a'+i)),
						"namespace": "default",
						"labels": map[string]interface{}{
							"test-group": timestamp,
							"test-index": string(rune('0' + i)),
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

		// List configmaps with label selector
		selector := "test-group=" + timestamp
		list, err := env.Client.ListResources(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, "default", selector)
		require.NoError(t, err)
		require.NotNil(t, list)

		// UnstructuredList has Items field directly
		assert.GreaterOrEqual(t, len(list.Items), 3, "Should find at least 3 configmaps")
	})
}

// TestIntegration_UpdateResource tests updating resources
func TestIntegration_UpdateResource(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("update configmap data", func(t *testing.T) {
		cmName := "test-update-cm-" + time.Now().Format("20060102150405")

		// Create configmap
		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      cmName,
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key1": "original-value",
				},
			},
		}
		cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)

		created, err := env.Client.CreateResource(env.Ctx, cm)
		require.NoError(t, err)

		// Update the data
		err = unstructured.SetNestedField(created.Object, "updated-value", "data", "key1")
		require.NoError(t, err)
		err = unstructured.SetNestedField(created.Object, "new-value", "data", "key2")
		require.NoError(t, err)

		updated, err := env.Client.UpdateResource(env.Ctx, created)
		require.NoError(t, err)
		require.NotNil(t, updated)

		// Verify the update
		data, found, err := unstructured.NestedStringMap(updated.Object, "data")
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "updated-value", data["key1"])
		assert.Equal(t, "new-value", data["key2"])
	})
}

// TestIntegration_DeleteResource tests deleting resources
func TestIntegration_DeleteResource(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("delete namespace", func(t *testing.T) {
		nsName := "test-delete-ns-" + time.Now().Format("20060102150405")

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

		_, err := env.Client.CreateResource(env.Ctx, ns)
		require.NoError(t, err)

		// Verify it exists
		_, err = env.Client.GetResource(env.Ctx, k8sclient.CommonResourceKinds.Namespace, "", nsName)
		require.NoError(t, err)

		// Delete the namespace
		err = env.Client.DeleteResource(env.Ctx, k8sclient.CommonResourceKinds.Namespace, "", nsName)
		require.NoError(t, err)

		// Verify it's being deleted (namespaces go into Terminating phase)
		time.Sleep(100 * time.Millisecond)
		deletedNs, err := env.Client.GetResource(env.Ctx, k8sclient.CommonResourceKinds.Namespace, "", nsName)
		if err == nil {
			// Namespace still exists, should have deletionTimestamp set (Terminating state)
			deletionTimestamp := deletedNs.GetDeletionTimestamp()
			assert.NotNil(t, deletionTimestamp, "Namespace should have deletionTimestamp set (Terminating state)")
		} else {
			// Namespace already deleted completely
			require.True(t, k8serrors.IsNotFound(err), "Expected NotFound error for deleted namespace")
		}
	})
}

// TestIntegration_ResourceLifecycle tests full CRUD lifecycle
func TestIntegration_ResourceLifecycle(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup(t)

	t.Run("full configmap lifecycle", func(t *testing.T) {
		cmName := "lifecycle-cm-" + time.Now().Format("20060102150405")

		// 1. Create
		cm := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      cmName,
					"namespace": "default",
					"labels": map[string]interface{}{
						"lifecycle": "test",
					},
				},
				"data": map[string]interface{}{
					"stage": "created",
				},
			},
		}
		cm.SetGroupVersionKind(k8sclient.CommonResourceKinds.ConfigMap)

		created, err := env.Client.CreateResource(env.Ctx, cm)
		require.NoError(t, err)
		assert.Equal(t, cmName, created.GetName())

		// 2. Get and verify
		retrieved, err := env.Client.GetResource(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, "default", cmName)
		require.NoError(t, err)
		data, _, _ := unstructured.NestedString(retrieved.Object, "data", "stage")
		assert.Equal(t, "created", data)

		// 3. Update
		err = unstructured.SetNestedField(retrieved.Object, "updated", "data", "stage")
		require.NoError(t, err)
		updated, err := env.Client.UpdateResource(env.Ctx, retrieved)
		require.NoError(t, err)
		data, _, _ = unstructured.NestedString(updated.Object, "data", "stage")
		assert.Equal(t, "updated", data)

		// 4. Get and verify update
		retrieved2, err := env.Client.GetResource(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, "default", cmName)
		require.NoError(t, err)
		data, _, _ = unstructured.NestedString(retrieved2.Object, "data", "stage")
		assert.Equal(t, "updated", data)

		// 5. Delete
		err = env.Client.DeleteResource(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, "default", cmName)
		require.NoError(t, err)

		// 6. Verify deletion
		_, err = env.Client.GetResource(env.Ctx, k8sclient.CommonResourceKinds.ConfigMap, "default", cmName)
		assert.Error(t, err)
	})
}
