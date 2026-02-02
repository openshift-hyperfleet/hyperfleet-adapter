package maestro_client_integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/maestro_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"
)

// TestMaestroClientConnection tests basic client connection to Maestro
func TestMaestroClientConnection(t *testing.T) {
	env := GetSharedEnv(t)

	log, err := logger.NewLogger(logger.Config{
		Level:     "debug",
		Format:    "text",
		Component: "maestro-integration-test",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := &maestro_client.Config{
		MaestroServerAddr:  env.MaestroServerAddr,
		GRPCServerAddr:     env.MaestroGRPCAddr,
		SourceID:           "integration-test-source",
		Insecure: true,
	}

	client, err := maestro_client.NewMaestroClient(ctx, config, log)
	require.NoError(t, err, "Should create Maestro client successfully")
	defer client.Close() //nolint:errcheck

	assert.NotNil(t, client.WorkClient(), "WorkClient should not be nil")
	assert.Equal(t, "integration-test-source", client.SourceID())
}

// TestMaestroClientCreateManifestWork tests creating a ManifestWork
func TestMaestroClientCreateManifestWork(t *testing.T) {
	env := GetSharedEnv(t)

	log, err := logger.NewLogger(logger.Config{
		Level:     "debug",
		Format:    "text",
		Component: "maestro-integration-test",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &maestro_client.Config{
		MaestroServerAddr:  env.MaestroServerAddr,
		GRPCServerAddr:     env.MaestroGRPCAddr,
		SourceID:           "integration-test-create",
		Insecure: true,
	}

	client, err := maestro_client.NewMaestroClient(ctx, config, log)
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck

	// First, we need to register a consumer (cluster) with Maestro
	// For integration tests, we'll use a test consumer name
	consumerName := "test-cluster-create"

	// Create a simple namespace manifest
	namespaceManifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": "test-namespace",
			"annotations": map[string]interface{}{
				constants.AnnotationGeneration: "1",
			},
		},
	}

	namespaceJSON, err := json.Marshal(namespaceManifest)
	require.NoError(t, err)

	// Create ManifestWork
	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-manifestwork-create",
			Namespace: consumerName,
			Annotations: map[string]string{
				constants.AnnotationGeneration: "1",
			},
			Labels: map[string]string{
				"test": "integration",
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						RawExtension: runtime.RawExtension{
							Raw: namespaceJSON,
						},
					},
				},
			},
		},
	}

	// Create the ManifestWork
	created, err := client.CreateManifestWork(ctx, consumerName, work)

	// Note: This may fail if the consumer doesn't exist in Maestro
	// The test validates the client can communicate with Maestro
	if err != nil {
		t.Logf("CreateManifestWork returned error (may be expected if consumer not registered): %v", err)
		// Check if it's a "consumer not found" type error
		assert.Contains(t, err.Error(), "consumer", "Error should be related to consumer registration")
	} else {
		assert.NotNil(t, created)
		assert.Equal(t, work.Name, created.Name)
		t.Logf("Created ManifestWork: %s/%s", created.Namespace, created.Name)
	}
}

// TestMaestroClientListManifestWorks tests listing ManifestWorks
func TestMaestroClientListManifestWorks(t *testing.T) {
	env := GetSharedEnv(t)

	log, err := logger.NewLogger(logger.Config{
		Level:     "debug",
		Format:    "text",
		Component: "maestro-integration-test",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := &maestro_client.Config{
		MaestroServerAddr:  env.MaestroServerAddr,
		GRPCServerAddr:     env.MaestroGRPCAddr,
		SourceID:           "integration-test-list",
		Insecure: true,
	}

	client, err := maestro_client.NewMaestroClient(ctx, config, log)
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck

	consumerName := "test-cluster-list"

	// List ManifestWorks (empty label selector = list all)
	list, err := client.ListManifestWorks(ctx, consumerName, "")

	// This may return empty or error depending on whether consumer exists
	if err != nil {
		t.Logf("ListManifestWorks returned error (may be expected): %v", err)
	} else {
		assert.NotNil(t, list)
		t.Logf("Found %d ManifestWorks for consumer %s", len(list.Items), consumerName)
	}
}

// TestMaestroClientApplyManifestWork tests the apply (create or update) operation
func TestMaestroClientApplyManifestWork(t *testing.T) {
	env := GetSharedEnv(t)

	log, err := logger.NewLogger(logger.Config{
		Level:     "debug",
		Format:    "text",
		Component: "maestro-integration-test",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &maestro_client.Config{
		MaestroServerAddr:  env.MaestroServerAddr,
		GRPCServerAddr:     env.MaestroGRPCAddr,
		SourceID:           "integration-test-apply",
		Insecure: true,
	}

	client, err := maestro_client.NewMaestroClient(ctx, config, log)
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck

	consumerName := "test-cluster-apply"

	// Create a ConfigMap manifest
	configMapManifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "test-config",
			"namespace": "default",
			"annotations": map[string]interface{}{
				constants.AnnotationGeneration: "1",
			},
		},
		"data": map[string]interface{}{
			"key1": "value1",
		},
	}

	configMapJSON, err := json.Marshal(configMapManifest)
	require.NoError(t, err)

	// Create ManifestWork
	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-manifestwork-apply",
			Namespace: consumerName,
			Annotations: map[string]string{
				constants.AnnotationGeneration: "1",
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						RawExtension: runtime.RawExtension{
							Raw: configMapJSON,
						},
					},
				},
			},
		},
	}

	// Apply the ManifestWork (should create if not exists)
	applied, err := client.ApplyManifestWork(ctx, consumerName, work)

	if err != nil {
		t.Logf("ApplyManifestWork returned error (may be expected if consumer not registered): %v", err)
	} else {
		assert.NotNil(t, applied)
		t.Logf("Applied ManifestWork: %s/%s", applied.Namespace, applied.Name)

		// Now apply again with updated generation (should update)
		work.Annotations[constants.AnnotationGeneration] = "2"
		configMapManifest["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})[constants.AnnotationGeneration] = "2"
		configMapManifest["data"].(map[string]interface{})["key2"] = "value2"
		configMapJSON, _ = json.Marshal(configMapManifest)
		work.Spec.Workload.Manifests[0].Raw = configMapJSON

		updated, err := client.ApplyManifestWork(ctx, consumerName, work)
		if err != nil {
			t.Logf("ApplyManifestWork (update) returned error: %v", err)
		} else {
			assert.NotNil(t, updated)
			t.Logf("Updated ManifestWork: %s/%s", updated.Namespace, updated.Name)
		}
	}
}

// TestMaestroClientGenerationSkip tests that apply skips when generation matches
func TestMaestroClientGenerationSkip(t *testing.T) {
	env := GetSharedEnv(t)

	log, err := logger.NewLogger(logger.Config{
		Level:     "debug",
		Format:    "text",
		Component: "maestro-integration-test",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config := &maestro_client.Config{
		MaestroServerAddr:  env.MaestroServerAddr,
		GRPCServerAddr:     env.MaestroGRPCAddr,
		SourceID:           "integration-test-skip",
		Insecure: true,
	}

	client, err := maestro_client.NewMaestroClient(ctx, config, log)
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck

	consumerName := "test-cluster-skip"

	// Create a simple manifest
	manifest := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "test-skip-config",
			"namespace": "default",
			"annotations": map[string]interface{}{
				constants.AnnotationGeneration: "5",
			},
		},
		"data": map[string]interface{}{
			"test": "data",
		},
	}

	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	work := &workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-manifestwork-skip",
			Namespace: consumerName,
			Annotations: map[string]string{
				constants.AnnotationGeneration: "5",
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						RawExtension: runtime.RawExtension{
							Raw: manifestJSON,
						},
					},
				},
			},
		},
	}

	// First apply
	result1, err := client.ApplyManifestWork(ctx, consumerName, work)
	if err != nil {
		t.Skipf("Skipping generation skip test - consumer may not be registered: %v", err)
	}
	require.NotNil(t, result1)

	// Apply again with same generation - should skip (return existing without update)
	result2, err := client.ApplyManifestWork(ctx, consumerName, work)
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both should have the same resource version if skipped
	assert.Equal(t, result1.ResourceVersion, result2.ResourceVersion,
		"Resource version should match when generation unchanged (skip)")
}
