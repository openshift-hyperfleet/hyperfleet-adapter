package executor

import (
	"context"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockAPIClient creates a new mock API client for convenience
func newMockAPIClient() *hyperfleet_api.MockClient {
	return hyperfleet_api.NewMockClient()
}

// TestNewExecutor tests the NewExecutor function
func TestNewExecutor(t *testing.T) {
	tests := []struct {
		name        string
		config      *ExecutorConfig
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "missing adapter config",
			config: &ExecutorConfig{
				APIClient: newMockAPIClient(),
				Logger:    logger.NewTestLogger(),
			},
			expectError: true,
		},
		{
			name: "missing API client",
			config: &ExecutorConfig{
				AdapterConfig: &config_loader.AdapterConfig{},
				Logger:        logger.NewTestLogger(),
			},
			expectError: true,
		},
		{
			name: "missing logger",
			config: &ExecutorConfig{
				AdapterConfig: &config_loader.AdapterConfig{},
				APIClient:     newMockAPIClient(),
			},
			expectError: true,
		},
		{
			name: "valid config",
			config: &ExecutorConfig{
				AdapterConfig: &config_loader.AdapterConfig{},
				APIClient:     newMockAPIClient(),
				K8sClient:     k8s_client.NewMockK8sClient(),
				Logger:        logger.NewTestLogger(),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExecutor(tt.config)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecutorBuilder(t *testing.T) {
	config := &config_loader.AdapterConfig{
		Metadata: config_loader.Metadata{
			Name:      "test-adapter",
			Namespace: "test-ns",
		},
	}

	exec, err := NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithK8sClient(k8s_client.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)
	require.NotNil(t, exec)
}

func TestExecutionContext(t *testing.T) {
	ctx := context.Background()
	eventData := map[string]interface{}{
		"cluster_id": "test-cluster",
	}

	execCtx := NewExecutionContext(ctx, eventData)

	assert.Equal(t, "test-cluster", execCtx.EventData["cluster_id"])
	assert.Empty(t, execCtx.Params)
	assert.Empty(t, execCtx.Resources)
	assert.Equal(t, string(StatusSuccess), execCtx.Adapter.ExecutionStatus)
}

func TestExecutionContext_SetError(t *testing.T) {
	ctx := context.Background()
	execCtx := NewExecutionContext(ctx, map[string]interface{}{})
	execCtx.SetError("TestReason", "Test message")

	assert.Equal(t, string(StatusFailed), execCtx.Adapter.ExecutionStatus)
	assert.Equal(t, "TestReason", execCtx.Adapter.ErrorReason)
	assert.Equal(t, "Test message", execCtx.Adapter.ErrorMessage)
}

func TestExecutorError(t *testing.T) {
	err := NewExecutorError("test-step", "test message", nil)

	expected := "test-step: test message"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}

	// With wrapped error
	wrappedErr := NewExecutorError("create", "failed to create", context.Canceled)
	assert.Equal(t, context.Canceled, wrappedErr.Unwrap())
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		data        map[string]interface{}
		expected    string
		expectError bool
	}{
		{
			name:     "simple variable",
			template: "Hello {{ .name }}!",
			data:     map[string]interface{}{"name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "no template",
			template: "plain text",
			data:     map[string]interface{}{},
			expected: "plain text",
		},
		{
			name:     "nested variable",
			template: "{{ .cluster.id }}",
			data: map[string]interface{}{
				"cluster": map[string]interface{}{"id": "test-123"},
			},
			expected: "test-123",
		},
		{
			name:        "missing variable",
			template:    "{{ .missing }}",
			data:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderTemplate(tt.template, tt.data)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestStepBasedExecution tests the step-based execution model
func TestStepBasedExecution_ParamSteps(t *testing.T) {
	// Set up environment variable for test
	t.Setenv("TEST_VAR", "test-value")

	config := &config_loader.AdapterConfig{
		Metadata: config_loader.Metadata{
			Name:      "test-adapter",
			Namespace: "test-ns",
		},
		Spec: config_loader.AdapterConfigSpec{
			Steps: []config_loader.Step{
				{
					Name: "testParam",
					Param: &config_loader.ParamStep{
						Source: "env.TEST_VAR",
					},
				},
				{
					Name: "eventParam",
					Param: &config_loader.ParamStep{
						Source: "event.cluster_id",
					},
				},
			},
		},
	}

	exec, err := NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithK8sClient(k8s_client.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)

	rawData := map[string]interface{}{
		"cluster_id": "cluster-456",
	}

	ctx := logger.WithEventID(context.Background(), "test-event-123")
	result := exec.Execute(ctx, rawData)

	// Check result
	assert.Equal(t, StatusSuccess, result.Status)
	assert.Equal(t, "test-value", result.Params["testParam"])
	assert.Equal(t, "cluster-456", result.Params["eventParam"])
}

func TestStepBasedExecution_WhenClause(t *testing.T) {
	config := &config_loader.AdapterConfig{
		Metadata: config_loader.Metadata{
			Name:      "test-adapter",
			Namespace: "test-ns",
		},
		Spec: config_loader.AdapterConfigSpec{
			Steps: []config_loader.Step{
				{
					Name: "status",
					Param: &config_loader.ParamStep{
						Value: "active",
					},
				},
				{
					Name: "activeLog",
					When: "status == 'active'",
					Log: &config_loader.LogStep{
						Message: "Status is active",
					},
				},
				{
					Name: "inactiveLog",
					When: "status == 'inactive'",
					Log: &config_loader.LogStep{
						Message: "Status is inactive",
					},
				},
			},
		},
	}

	exec, err := NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithK8sClient(k8s_client.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)

	ctx := logger.WithEventID(context.Background(), "test-event-when")
	result := exec.Execute(ctx, nil)

	// Check result
	assert.Equal(t, StatusSuccess, result.Status)

	// Verify step results
	require.Len(t, result.StepResults, 3)

	// First param step should succeed
	assert.Equal(t, "status", result.StepResults[0].Name)
	assert.False(t, result.StepResults[0].Skipped)
	assert.Nil(t, result.StepResults[0].Error)

	// Active log should execute (when clause is true)
	assert.Equal(t, "activeLog", result.StepResults[1].Name)
	assert.False(t, result.StepResults[1].Skipped)

	// Inactive log should be skipped (when clause is false)
	assert.Equal(t, "inactiveLog", result.StepResults[2].Name)
	assert.True(t, result.StepResults[2].Skipped)
}

func TestStepBasedExecution_LogStep(t *testing.T) {
	config := &config_loader.AdapterConfig{
		Metadata: config_loader.Metadata{
			Name:      "test-adapter",
			Namespace: "test-ns",
		},
		Spec: config_loader.AdapterConfigSpec{
			Steps: []config_loader.Step{
				{
					Name: "logMessage",
					Log: &config_loader.LogStep{
						Level:   "info",
						Message: "Test log message",
					},
				},
			},
		},
	}

	exec, err := NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithK8sClient(k8s_client.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)

	ctx := logger.WithEventID(context.Background(), "test-event-log")
	result := exec.Execute(ctx, nil)

	assert.Equal(t, StatusSuccess, result.Status)
	require.Len(t, result.StepResults, 1)
	assert.Equal(t, "logMessage", result.StepResults[0].Name)
	assert.Equal(t, StepTypeLog, result.StepResults[0].Type)
	assert.False(t, result.StepResults[0].Skipped)
}

func TestStepBasedExecution_PayloadStep(t *testing.T) {
	config := &config_loader.AdapterConfig{
		Metadata: config_loader.Metadata{
			Name:      "test-adapter",
			Namespace: "test-ns",
		},
		Spec: config_loader.AdapterConfigSpec{
			Steps: []config_loader.Step{
				{
					Name: "status",
					Param: &config_loader.ParamStep{
						Value: "ready",
					},
				},
				{
					Name: "statusPayload",
					Payload: map[string]interface{}{
						"status": map[string]interface{}{
							"field": "status",
						},
						"timestamp": "2024-01-01",
					},
				},
			},
		},
	}

	exec, err := NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithK8sClient(k8s_client.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)

	ctx := logger.WithEventID(context.Background(), "test-event-payload")
	result := exec.Execute(ctx, nil)

	assert.Equal(t, StatusSuccess, result.Status)
	require.Len(t, result.StepResults, 2)

	// Check payload step result
	payloadResult := result.StepResults[1]
	assert.Equal(t, "statusPayload", payloadResult.Name)
	assert.Equal(t, StepTypePayload, payloadResult.Type)

	// The result should be the built payload
	payload, ok := payloadResult.Result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ready", payload["status"])
	assert.Equal(t, "2024-01-01", payload["timestamp"])
}

func TestStepBasedExecution_SoftFailure(t *testing.T) {
	config := &config_loader.AdapterConfig{
		Metadata: config_loader.Metadata{
			Name:      "test-adapter",
			Namespace: "test-ns",
		},
		Spec: config_loader.AdapterConfigSpec{
			Steps: []config_loader.Step{
				{
					Name: "step1",
					Param: &config_loader.ParamStep{
						Value: "value1",
					},
				},
				{
					Name: "failingStep",
					Param: &config_loader.ParamStep{
						Source: "env.NONEXISTENT_VAR", // This will use default or be nil
					},
				},
				{
					Name: "step3",
					Param: &config_loader.ParamStep{
						Value: "value3",
					},
				},
			},
		},
	}

	exec, err := NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithK8sClient(k8s_client.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)

	ctx := logger.WithEventID(context.Background(), "test-event-soft")
	result := exec.Execute(ctx, nil)

	// Execution should still succeed (soft failure model)
	assert.Equal(t, StatusSuccess, result.Status)

	// All steps should have been executed
	require.Len(t, result.StepResults, 3)

	// First step should succeed
	assert.Equal(t, "step1", result.StepResults[0].Name)
	assert.Nil(t, result.StepResults[0].Error)

	// Missing env var returns nil, not an error (soft behavior)
	assert.Equal(t, "failingStep", result.StepResults[1].Name)

	// Third step should still execute
	assert.Equal(t, "step3", result.StepResults[2].Name)
	assert.Nil(t, result.StepResults[2].Error)
}

func TestStepBasedExecution_EmptySteps(t *testing.T) {
	config := &config_loader.AdapterConfig{
		Metadata: config_loader.Metadata{
			Name:      "test-adapter",
			Namespace: "test-ns",
		},
		Spec: config_loader.AdapterConfigSpec{
			Steps: []config_loader.Step{},
		},
	}

	exec, err := NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithK8sClient(k8s_client.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)

	ctx := logger.WithEventID(context.Background(), "test-event-empty")
	result := exec.Execute(ctx, nil)

	assert.Equal(t, StatusSuccess, result.Status)
	assert.Empty(t, result.StepResults)
}
