package config_loader_integration

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getProjectRoot returns the absolute path to the project root.
// This file is located at test/integration/config-loader/loader_template_test.go,
// so we go up 3 levels to reach the project root.
func getProjectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "../../..")
}

// TestLoadTemplateConfig tests loading the actual adapter-config-template.yaml
// This is an integration test that validates the shipped template configuration.
func TestLoadTemplateConfig(t *testing.T) {
	projectRoot := getProjectRoot()
	configPath := filepath.Join(projectRoot, "configs/adapter-config-template.yaml")

	// Use WithBaseDir to resolve relative paths in the config (e.g., buildRef, manifest.ref)
	baseDir := filepath.Dir(configPath)
	config, err := config_loader.Load(configPath, config_loader.WithBaseDir(baseDir))
	require.NoError(t, err, "should be able to load template config")
	require.NotNil(t, config)

	// Verify basic structure
	assert.Equal(t, "hyperfleet.redhat.com/v1alpha1", config.APIVersion)
	assert.Equal(t, "AdapterConfig", config.Kind)
	assert.Equal(t, "example-adapter", config.Metadata.Name)
	assert.Equal(t, "hyperfleet-system", config.Metadata.Namespace)

	// Verify adapter info
	assert.Equal(t, "0.0.1", config.Spec.Adapter.Version)

	// Verify HyperFleet API config
	assert.Equal(t, "2s", config.Spec.HyperfleetAPI.Timeout)
	assert.Equal(t, 3, config.Spec.HyperfleetAPI.RetryAttempts)
	assert.Equal(t, "exponential", config.Spec.HyperfleetAPI.RetryBackoff)

	// Verify params exist
	assert.NotEmpty(t, config.Spec.Params)
	assert.GreaterOrEqual(t, len(config.Spec.Params), 5, "should have at least 5 parameters")

	// Check specific params (using accessor method)
	clusterIdParam := config.GetParamByName("clusterId")
	require.NotNil(t, clusterIdParam, "clusterId parameter should exist")
	assert.Equal(t, "event.cluster_id", clusterIdParam.Source)
	assert.True(t, clusterIdParam.Required)

	// Verify preconditions
	assert.NotEmpty(t, config.Spec.Preconditions)
	assert.GreaterOrEqual(t, len(config.Spec.Preconditions), 1, "should have at least 1 precondition")

	// Check first precondition
	firstPrecond := config.Spec.Preconditions[0]
	assert.Equal(t, "clusterStatus", firstPrecond.Name)
	assert.NotNil(t, firstPrecond.APICall)
	assert.Equal(t, "GET", firstPrecond.APICall.Method)
	assert.Equal(t, "clusterDetails", firstPrecond.StoreResponseAs)
	assert.NotEmpty(t, firstPrecond.Extract)
	assert.NotEmpty(t, firstPrecond.Conditions)

	// Verify extracted fields
	clusterNameExtract := findExtractByAs(firstPrecond.Extract, "clusterName")
	require.NotNil(t, clusterNameExtract)
	assert.Equal(t, "metadata.name", clusterNameExtract.Field)

	// Verify conditions in precondition
	assert.GreaterOrEqual(t, len(firstPrecond.Conditions), 1)
	firstCondition := firstPrecond.Conditions[0]
	assert.Equal(t, "clusterPhase", firstCondition.Field)
	assert.Equal(t, "in", firstCondition.Operator)

	// Verify resources
	assert.NotEmpty(t, config.Spec.Resources)
	assert.GreaterOrEqual(t, len(config.Spec.Resources), 1, "should have at least 1 resource")

	// Check first resource
	firstResource := config.Spec.Resources[0]
	assert.Equal(t, "clusterNamespace", firstResource.Name)
	assert.NotNil(t, firstResource.Manifest)
	assert.NotNil(t, firstResource.Discovery)

	// Verify post configuration
	if config.Spec.Post != nil {
		assert.NotEmpty(t, config.Spec.Post.Params)
		assert.NotEmpty(t, config.Spec.Post.PostActions)

		// Check post action
		if len(config.Spec.Post.PostActions) > 0 {
			firstAction := config.Spec.Post.PostActions[0]
			assert.NotEmpty(t, firstAction.Name)
			if firstAction.APICall != nil {
				assert.NotEmpty(t, firstAction.APICall.Method)
				assert.NotEmpty(t, firstAction.APICall.URL)
			}
		}
	}
}

// TestLoadValidTestConfig tests loading the valid test config
func TestLoadValidTestConfig(t *testing.T) {
	projectRoot := getProjectRoot()
	configPath := filepath.Join(projectRoot, "test/testdata/adapter_config_valid.yaml")

	baseDir := filepath.Dir(configPath)
	config, err := config_loader.Load(configPath, config_loader.WithBaseDir(baseDir))
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, "hyperfleet.redhat.com/v1alpha1", config.APIVersion)
	assert.Equal(t, "AdapterConfig", config.Kind)
	assert.Equal(t, "example-adapter", config.Metadata.Name)
}

// Helper function to find an extract field by "as" name
func findExtractByAs(extracts []config_loader.ExtractField, as string) *config_loader.ExtractField {
	for i := range extracts {
		if extracts[i].As == as {
			return &extracts[i]
		}
	}
	return nil
}

