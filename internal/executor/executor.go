package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	pkgotel "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// NewExecutor creates a new Executor with the given configuration
func NewExecutor(config *ExecutorConfig) (*Executor, error) {
	if err := validateExecutorConfig(config); err != nil {
		return nil, err
	}

	log := config.Logger

	// Create phases (each phase contains its own business logic)
	paramExtractionPhase := NewParamExtractionPhase(config.Config, config.K8sClient, log)
	preconditionsPhase := NewPreconditionsPhase(config.APIClient, config.Config, log)
	resourcesPhase := NewResourcesPhase(config.K8sClient, config.Config, log)
	postActionsPhase := NewPostActionsPhase(config.APIClient, config.Config, log)

	// Create pipeline with all phases
	pipeline := NewPipeline(log,
		paramExtractionPhase,
		preconditionsPhase,
		resourcesPhase,
		postActionsPhase,
	)

	return &Executor{
		config:               config,
		log:                  log,
		pipeline:             pipeline,
		paramExtractionPhase: paramExtractionPhase,
		preconditionsPhase:   preconditionsPhase,
		resourcesPhase:       resourcesPhase,
		postActionsPhase:     postActionsPhase,
	}, nil
}
func validateExecutorConfig(config *ExecutorConfig) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}

	requiredFields := []string{
		"Config",
		"APIClient",
		"Logger",
		"K8sClient"}

	for _, field := range requiredFields {
		if reflect.ValueOf(config).Elem().FieldByName(field).IsNil() {
			return fmt.Errorf("field %s is required", field)
		}
	}
	return nil
}

// Execute processes event data according to the adapter configuration
// The caller is responsible for:
// - Adding event ID to context for logging correlation using logger.WithEventID()
func (e *Executor) Execute(ctx context.Context, data interface{}) *ExecutionResult {
	// Start OTel span and add trace context to logs
	ctx, span := e.startTracedExecution(ctx)
	defer span.End()

	// Parse event data
	eventData, rawData, err := ParseEventData(data)
	if err != nil {
		parseErr := fmt.Errorf("failed to parse event data: %w", err)
		errCtx := logger.WithErrorField(ctx, parseErr)
		e.log.Errorf(errCtx, "Failed to parse event data")
		return &ExecutionResult{
			Status:       StatusFailed,
			CurrentPhase: PhaseParamExtraction,
			Errors:       map[ExecutionPhase]error{PhaseParamExtraction: parseErr},
		}
	}

	// This is intended to set OwnerReference and ResourceID for the event when it exist
	// For example, when a NodePool event arrived
	// the logger will set the cluster_id=owner_id, nodepool_id=resource_id, resource_type=nodepool
	// but when a resource is cluster type, it will just record cluster_id=resource_id
	if eventData.OwnedReference != nil {
		ctx = logger.WithResourceType(ctx, eventData.Kind)
		ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
		ctx = logger.WithDynamicResourceID(ctx, eventData.OwnedReference.Kind, eventData.OwnedReference.ID)
	} else {
		ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
	}

	execCtx := NewExecutionContext(ctx, rawData, e.config.Config)

	e.log.Info(ctx, "Processing event")

	// Execute all phases through the pipeline
	phaseResults := e.pipeline.Execute(ctx, execCtx)

	// Build execution result from phase results
	result := e.buildExecutionResult(phaseResults, execCtx)

	// Log final status
	if result.Status == StatusSuccess {
		e.log.Infof(ctx, "Event execution finished: event_execution_status=success resources_skipped=%t reason=%s", result.ResourcesSkipped, result.SkipReason)
	} else {
		// Combine all errors into a single error for logging
		var errMsgs []string
		for phase, err := range result.Errors {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", phase, err))
		}
		combinedErr := fmt.Errorf("execution failed: %s", strings.Join(errMsgs, "; "))
		errCtx := logger.WithErrorField(ctx, combinedErr)
		e.log.Errorf(errCtx, "Event execution finished: event_execution_status=failed")
	}

	return result
}

// buildExecutionResult converts phase results into the final ExecutionResult
func (e *Executor) buildExecutionResult(phaseResults []PhaseResult, execCtx *ExecutionContext) *ExecutionResult {
	result := &ExecutionResult{
		Status:           StatusSuccess,
		Params:           execCtx.Params,
		Errors:           make(map[ExecutionPhase]error),
		ExecutionContext: execCtx,
	}

	// Track the last phase that ran
	for _, pr := range phaseResults {
		result.CurrentPhase = pr.Phase

		if pr.Error != nil {
			result.Status = StatusFailed
			result.Errors[pr.Phase] = pr.Error
		}

		if pr.Skipped && pr.Phase == PhaseResources {
			result.ResourcesSkipped = true
			result.SkipReason = pr.SkipReason
		}
	}

	// Collect results from individual phases
	if e.preconditionsPhase != nil {
		result.PreconditionResults = e.preconditionsPhase.Results()
		// Update skip reason from preconditions outcome if available
		if outcome := e.preconditionsPhase.Outcome(); outcome != nil && !outcome.AllMatched {
			result.ResourcesSkipped = true
			result.SkipReason = outcome.NotMetReason
		}
	}

	if e.resourcesPhase != nil {
		result.ResourceResults = e.resourcesPhase.Results()
	}

	if e.postActionsPhase != nil {
		result.PostActionResults = e.postActionsPhase.Results()
	}

	return result
}

// startTracedExecution creates an OTel span and adds trace context to logs.
// Returns the enriched context and span. Caller must call span.End() when done.
//
// This method:
//   - Creates an OTel span with trace_id and span_id (for distributed tracing)
//   - Adds trace_id and span_id to logger context (for log correlation)
//   - The trace context is automatically propagated to outgoing HTTP requests
func (e *Executor) startTracedExecution(ctx context.Context) (context.Context, trace.Span) {
	componentName := e.config.Config.Metadata.Name
	ctx, span := otel.Tracer(componentName).Start(ctx, "Execute")

	// Add trace_id and span_id to logger context for log correlation
	ctx = logger.WithOTelTraceContext(ctx)

	return ctx, span
}

// CreateHandler creates an event handler function that can be used with the broker subscriber
// This is a convenience method for integrating with the broker_consumer package
//
// Error handling strategy:
// - All failures are logged but the message is ACKed (return nil)
// - This prevents infinite retry loops for non-recoverable errors (e.g., 400 Bad Request, invalid data)
func (e *Executor) CreateHandler() func(ctx context.Context, evt *event.Event) error {
	return func(ctx context.Context, evt *event.Event) error {
		// Add event ID to context for logging correlation
		ctx = logger.WithEventID(ctx, evt.ID())

		// Extract W3C trace context from CloudEvent extensions (if present)
		// This enables distributed tracing when upstream services (e.g., Sentinel)
		// include traceparent/tracestate in the CloudEvent
		ctx = pkgotel.ExtractTraceContextFromCloudEvent(ctx, evt)

		// Log event metadata
		e.log.Infof(ctx, "Event received: id=%s type=%s source=%s time=%s",
			evt.ID(), evt.Type(), evt.Source(), evt.Time())

		_ = e.Execute(ctx, evt.Data())

		e.log.Infof(ctx, "Event processed: type=%s source=%s time=%s",
			evt.Type(), evt.Source(), evt.Time())

		return nil
	}
}

// ParseEventData parses event data from various input types into structured EventData and raw map.
// Accepts: []byte (JSON), map[string]interface{}, or any JSON-serializable type.
// Returns: structured EventData, raw map for flexible access, and any error.
func ParseEventData(data interface{}) (*EventData, map[string]interface{}, error) {
	if data == nil {
		return &EventData{}, make(map[string]interface{}), nil
	}

	var jsonBytes []byte
	var err error

	switch v := data.(type) {
	case []byte:
		if len(v) == 0 {
			return &EventData{}, make(map[string]interface{}), nil
		}
		jsonBytes = v
	case map[string]interface{}:
		// Already a map, marshal to JSON for struct conversion
		jsonBytes, err = json.Marshal(v)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal map data: error=%w", err)
		}
	default:
		// Try to marshal any other type
		jsonBytes, err = json.Marshal(v)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal data: error=%w", err)
		}
	}

	// Parse into structured EventData
	var eventData EventData
	if err := json.Unmarshal(jsonBytes, &eventData); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal to EventData: error=%w", err)
	}

	// Parse into raw map for flexible access
	var rawData map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &rawData); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal to map: error=%w", err)
	}

	return &eventData, rawData, nil
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

// WithConfig sets the unified configuration
func (b *ExecutorBuilder) WithConfig(config *config_loader.Config) *ExecutorBuilder {
	b.config.Config = config
	return b
}

// WithAPIClient sets the HyperFleet API client
func (b *ExecutorBuilder) WithAPIClient(client hyperfleet_api.Client) *ExecutorBuilder {
	b.config.APIClient = client
	return b
}

// WithK8sClient sets the Kubernetes client
func (b *ExecutorBuilder) WithK8sClient(client k8s_client.K8sClient) *ExecutorBuilder {
	b.config.K8sClient = client
	return b
}

// WithLogger sets the logger
func (b *ExecutorBuilder) WithLogger(log logger.Logger) *ExecutorBuilder {
	b.config.Logger = log
	return b
}

// Build creates the Executor
func (b *ExecutorBuilder) Build() (*Executor, error) {
	return NewExecutor(b.config)
}
