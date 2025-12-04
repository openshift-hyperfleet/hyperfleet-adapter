package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

// NewExecutor creates a new Executor with the given configuration
func NewExecutor(config *ExecutorConfig) (*Executor, error) {
	if config == nil {
		return nil, NewExecutorError(PhaseParamExtraction, "init", "executor config is required", nil)
	}
	if config.AdapterConfig == nil {
		return nil, NewExecutorError(PhaseParamExtraction, "init", "adapter config is required", nil)
	}
	if config.APIClient == nil {
		return nil, NewExecutorError(PhaseParamExtraction, "init", "API client is required", nil)
	}
	if config.Logger == nil {
		return nil, NewExecutorError(PhaseParamExtraction, "init", "logger is required", nil)
	}

	return &Executor{
		config:             config,
		precondExecutor:    NewPreconditionExecutor(config.APIClient),
		resourceExecutor:   NewResourceExecutor(config.K8sClient, config.DryRun),
		postActionExecutor: NewPostActionExecutor(config.APIClient),
	}, nil
}

// Execute processes a CloudEvent according to the adapter configuration
// This is the main entry point for event processing
func (e *Executor) Execute(ctx context.Context, evt *event.Event) *ExecutionResult {
	// ============================================================================
	// Setup
	// ============================================================================
	if evt == nil {
		return &ExecutionResult{
			Status:      StatusFailed,
			Error:       NewExecutorError(PhaseParamExtraction, "init", "event is required", nil),
			ErrorReason: "nil event received",
		}
	}
	ctxWithEventID := context.WithValue(ctx, logger.EvtIDKey, evt.ID())
	eventLogger := logger.WithEventID(e.config.Logger, evt.ID())

	// Parse event data at the boundary (decouples CloudEvent from parameter extraction)
	eventData, err := parseEventData(evt)
	if err != nil {
		return &ExecutionResult{
			EventID:     evt.ID(),
			Status:      StatusFailed,
			Phase:       PhaseParamExtraction,
			Error:       NewExecutorError(PhaseParamExtraction, "parse_event", "failed to parse event data", err),
			ErrorReason: "event data parsing failed",
		}
	}

	execCtx := NewExecutionContext(ctxWithEventID, evt, eventData)

	result := &ExecutionResult{
		EventID: evt.ID(),
		Status:  StatusSuccess,
		Params:  make(map[string]interface{}),
	}

	eventLogger.Infof("Starting event execution: id=%s", evt.ID())

	// ============================================================================
	// Phase 1: Parameter Extraction
	// ============================================================================
	result.Phase = PhaseParamExtraction
	if err := e.executeParamExtraction(execCtx); err != nil {
		return e.finishWithError(result, execCtx, err, "parameter extraction failed", eventLogger)
	}
	result.Params = execCtx.Params
	eventLogger.Infof("Parameter extraction completed: extracted %d params", len(execCtx.Params))

	// ============================================================================
	// Phase 2: Preconditions
	// ============================================================================
	result.Phase = PhasePreconditions
	precondOutcome := e.precondExecutor.ExecuteAll(ctxWithEventID, e.config.AdapterConfig.Spec.Preconditions, execCtx, eventLogger)
	result.PreconditionResults = precondOutcome.Results

	if precondOutcome.Error != nil {
		// Process execution error: precondition evaluation failed
		result.Status = StatusFailed
		result.Error = precondOutcome.Error
		result.ErrorReason = "precondition evaluation failed"
		execCtx.SetError("PreconditionFailed", precondOutcome.Error.Error())
		eventLogger.Error(fmt.Sprintf("Precondition execution failed: %v", precondOutcome.Error))
		// Continue to post actions for error reporting
	} else if !precondOutcome.AllMatched {
		// Business outcome: precondition not satisfied
		result.ResourcesSkipped = true
		result.SkipReason = precondOutcome.NotMetReason
		execCtx.SetSkipped("PreconditionNotMet", precondOutcome.NotMetReason)
		eventLogger.Infof("Preconditions not met, resources will be skipped: %s", precondOutcome.NotMetReason)
	} else {
		// All preconditions matched
		eventLogger.Infof("Preconditions completed: %d preconditions evaluated", len(precondOutcome.Results))
	}

	// ============================================================================
	// Phase 3: Resources (skip if preconditions not met or previous error)
	// ============================================================================
	result.Phase = PhaseResources
	if result.Status == StatusSuccess && !result.ResourcesSkipped {
		resourceResults, err := e.resourceExecutor.ExecuteAll(ctxWithEventID, e.config.AdapterConfig.Spec.Resources, execCtx, eventLogger)
		result.ResourceResults = resourceResults

		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			result.ErrorReason = "resource execution failed"
			execCtx.SetError("ResourceFailed", err.Error())
			eventLogger.Error(fmt.Sprintf("Resource execution failed: %v", err))
			// Continue to post actions for error reporting
		} else {
			eventLogger.Infof("Resources completed: %d resources processed", len(resourceResults))
		}
	} else if result.ResourcesSkipped {
		eventLogger.Infof("Resources skipped: %s", result.SkipReason)
	} else if result.Status == StatusFailed {
		eventLogger.Infof("Resources skipped due to previous error")
	}

	// ============================================================================
	// Phase 4: Post Actions (always execute for error reporting)
	// ============================================================================
	result.Phase = PhasePostActions
	postResults, err := e.postActionExecutor.ExecuteAll(ctxWithEventID, e.config.AdapterConfig.Spec.Post, execCtx, eventLogger)
	result.PostActionResults = postResults

	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		result.ErrorReason = "post action execution failed"
		eventLogger.Error(fmt.Sprintf("Post action execution failed: %v", err))
	} else {
		eventLogger.Infof("Post actions completed: %d actions executed", len(postResults))
	}

	// ============================================================================
	// Finalize
	// ============================================================================
	result.ExecutionContext = execCtx

	// Final logging
	if result.Status == StatusSuccess {
		if result.ResourcesSkipped {
			eventLogger.Infof("Event execution completed successfully (resources skipped): id=%s reason=%s",
				evt.ID(), result.SkipReason)
		} else {
			eventLogger.Infof("Event execution completed successfully: id=%s",
				evt.ID())
		}
	} else {
		eventLogger.Error(fmt.Sprintf("Event execution failed: id=%s phase=%s reason=%s",
			evt.ID(), result.Phase, result.ErrorReason))
	}

	return result
}

// finishWithError is a helper to handle early termination with error
func (e *Executor) finishWithError(result *ExecutionResult, execCtx *ExecutionContext, err error, reason string, eventLogger logger.Logger) *ExecutionResult {
	result.Status = StatusFailed
	result.Error = err
	result.ErrorReason = reason
	result.ExecutionContext = execCtx
	result.Params = execCtx.Params
	eventLogger.Error(fmt.Sprintf("Event execution failed: id=%s phase=%s reason=%s",
		result.EventID, result.Phase, result.ErrorReason))
	return result
}

// executeParamExtraction extracts parameters from the event and environment
func (e *Executor) executeParamExtraction(execCtx *ExecutionContext) error {
	// Extract configured parameters
	if err := extractConfigParams(e.config.AdapterConfig, execCtx); err != nil {
		return err
	}

	// Add metadata params
	addMetadataParams(e.config.AdapterConfig, execCtx)

	return nil
}

// CreateHandler creates an event handler function that can be used with the broker subscriber
// This is a convenience method for integrating with the broker_consumer package
func (e *Executor) CreateHandler() func(ctx context.Context, evt *event.Event) error {
	return func(ctx context.Context, evt *event.Event) error {
		result := e.Execute(ctx, evt)

		if result.Status == StatusFailed {
			// Don't NACK for param extraction failures (invalid events should not be retried)
			if result.Phase == PhaseParamExtraction {
				return nil // ACK the event
			}
			return result.Error
		}

		// StatusSkipped is not an error - preconditions not met is expected behavior
		return nil
	}
}


// parseEventData parses the CloudEvent data payload into a map
// This is done at the boundary to decouple CloudEvent from parameter extraction
func parseEventData(evt *event.Event) (map[string]interface{}, error) {
	if evt == nil {
		return make(map[string]interface{}), nil
	}

	data := evt.Data()
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}

	var eventData map[string]interface{}
	if err := json.Unmarshal(data, &eventData); err != nil {
		return nil, fmt.Errorf("failed to parse event data as JSON: %w", err)
	}

	return eventData, nil
}

// ExecutorBuilder provides a fluent interface for building an Executor
type ExecutorBuilder struct {
	config *ExecutorConfig
}

// NewBuilder creates a new ExecutorBuilder
func NewBuilder() *ExecutorBuilder {
	return &ExecutorBuilder{
		config: &ExecutorConfig{},
	}
}

// WithAdapterConfig sets the adapter configuration
func (b *ExecutorBuilder) WithAdapterConfig(config *config_loader.AdapterConfig) *ExecutorBuilder {
	b.config.AdapterConfig = config
	return b
}

// WithAPIClient sets the HyperFleet API client
func (b *ExecutorBuilder) WithAPIClient(client hyperfleet_api.Client) *ExecutorBuilder {
	b.config.APIClient = client
	return b
}

// WithK8sClient sets the Kubernetes client
func (b *ExecutorBuilder) WithK8sClient(client *k8s_client.Client) *ExecutorBuilder {
	b.config.K8sClient = client
	return b
}

// WithLogger sets the logger
func (b *ExecutorBuilder) WithLogger(log logger.Logger) *ExecutorBuilder {
	b.config.Logger = log
	return b
}

// WithDryRun enables dry run mode
func (b *ExecutorBuilder) WithDryRun(dryRun bool) *ExecutorBuilder {
	b.config.DryRun = dryRun
	return b
}

// Build creates the Executor
func (b *ExecutorBuilder) Build() (*Executor, error) {
	return NewExecutor(b.config)
}

