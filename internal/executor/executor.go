package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

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

	return &Executor{
		config:       config,
		stepExecutor: NewStepExecutor(config.APIClient, config.K8sClient, config.Logger),
		log:          config.Logger,
	}, nil
}

func validateExecutorConfig(config *ExecutorConfig) error {
	if config == nil {
		return fmt.Errorf("config is required")
	}

	requiredFields := []string{
		"AdapterConfig",
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

// Execute processes pre-parsed event data according to the adapter configuration.
// The caller is responsible for:
// - Parsing the event data using ParseEventData()
// - Adding event ID to context for logging correlation using logger.WithEventID()
// - Adding logging context from EventData when available
//
// Parameters:
// - rawData: raw map for step execution and template rendering
func (e *Executor) Execute(ctx context.Context, rawData map[string]interface{}) *ExecutionResult {
	// Start OTel span and add trace context to logs
	ctx, span := e.startTracedExecution(ctx)
	defer span.End()

	// Ensure rawData is not nil
	if rawData == nil {
		rawData = make(map[string]interface{})
	}

	// Execute using step-based model
	e.log.Info(ctx, "Processing event")

	// Create metadata map for step context
	metadata := map[string]interface{}{
		"name":      e.config.AdapterConfig.Metadata.Name,
		"namespace": e.config.AdapterConfig.Metadata.Namespace,
		"labels":    e.config.AdapterConfig.Metadata.Labels,
	}

	// Create step execution context
	stepCtx := NewStepExecutionContext(ctx, rawData, metadata)

	// Execute all steps
	stepResult := e.stepExecutor.ExecuteAll(ctx, e.config.AdapterConfig.Spec.Steps, stepCtx)

	// Convert step result to execution result for compatibility
	result := &ExecutionResult{
		Status:            stepResult.Status,
		Params:            stepResult.Variables,
		Errors:            make(map[string]error),
		StepResults:       stepResult.StepResults,
		stepResultsByName: stepResult.StepResultsByName,
	}

	// Log completion
	if result.Status == StatusSuccess {
		e.log.Infof(ctx, "Event execution finished: event_execution_status=success step_count=%d", len(stepResult.StepResults))
	} else {
		errMsg := "unknown error"
		if stepResult.FirstError != nil {
			errMsg = fmt.Sprintf("%s: %s", stepResult.FirstError.Reason, stepResult.FirstError.Message)
		}
		e.log.Errorf(ctx, "Event execution finished: event_execution_status=failed error=%s", errMsg)
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
	componentName := e.config.AdapterConfig.Metadata.Name
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

		// Parse event data
		eventData, rawData, err := ParseEventData(evt.Data())
		if err != nil {
			parseErr := fmt.Errorf("failed to parse event data: %w", err)
			errCtx := logger.WithErrorField(ctx, parseErr)
			e.log.Errorf(errCtx, "Failed to parse event data")
			// ACK the message to prevent retry loops for parse errors
			return nil
		}

		// Set logging context from event data
		if eventData != nil {
			// This is intended to set OwnerReference and ResourceID for the event when it exist
			// For example, when a NodePool event arrived
			// the logger will set the cluster_id=owner_id, nodepool_id=resource_id, resource_type=nodepool
			// but when a resource is cluster type, it will just record cluster_id=resource_id
			if eventData.OwnedReference != nil {
				ctx = logger.WithResourceType(ctx, eventData.Kind)
				ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
				ctx = logger.WithDynamicResourceID(ctx, eventData.OwnedReference.Kind, eventData.OwnedReference.ID)
			} else if eventData.Kind != "" {
				ctx = logger.WithDynamicResourceID(ctx, eventData.Kind, eventData.ID)
			}
		}

		_ = e.Execute(ctx, rawData)

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
