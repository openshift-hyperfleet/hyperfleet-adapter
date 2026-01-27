package executor

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ExecutionStatus represents the status of execution (runtime perspective)
type ExecutionStatus string

const (
	// StatusSuccess indicates successful execution (adapter ran successfully)
	StatusSuccess ExecutionStatus = "success"
	// StatusFailed indicates failed execution (process execution error: API timeout, parse error, K8s error, etc.)
	StatusFailed ExecutionStatus = "failed"
)

// ResourceRef represents a reference to a HyperFleet resource
type ResourceRef struct {
	ID   string `json:"id,omitempty"`
	Kind string `json:"kind,omitempty"`
	Href string `json:"href,omitempty"`
}

// EventData represents the data payload of a HyperFleet CloudEvent
type EventData struct {
	ID             string       `json:"id,omitempty"`
	Kind           string       `json:"kind,omitempty"`
	Href           string       `json:"href,omitempty"`
	Generation     int64        `json:"generation,omitempty"`
	OwnedReference *ResourceRef `json:"owned_reference,omitempty"`
}

// ExecutorConfig holds configuration for the executor
type ExecutorConfig struct {
	// AdapterConfig is the loaded adapter configuration
	AdapterConfig *config_loader.AdapterConfig
	// APIClient is the HyperFleet API client
	APIClient hyperfleet_api.Client
	// K8sClient is the Kubernetes client
	K8sClient k8s_client.K8sClient
	// Logger is the logger instance
	Logger logger.Logger
}

// Executor processes CloudEvents according to the adapter configuration.
// Uses step-based execution model with sequential steps and 'when' clauses.
type Executor struct {
	config       *ExecutorConfig
	stepExecutor *StepExecutor
	log          logger.Logger
}

// ExecutionResult contains the result of processing an event
type ExecutionResult struct {
	// Status is the overall execution status (runtime perspective)
	Status ExecutionStatus
	// Params contains the extracted parameters and step results
	Params map[string]interface{}
	// Errors contains errors keyed by step name or error type
	Errors map[string]error
	// StepResults contains results of all executed steps in order
	StepResults []*StepResult
	// stepResultsByName provides quick lookup of step results by name (internal)
	stepResultsByName map[string]*StepResult
}

// GetStepResult returns a step result by name, or nil if not found
func (r *ExecutionResult) GetStepResult(name string) *StepResult {
	if r.stepResultsByName != nil {
		return r.stepResultsByName[name]
	}
	// Fallback to linear search if map not populated
	for _, sr := range r.StepResults {
		if sr.Name == name {
			return sr
		}
	}
	return nil
}

// ResourceResult contains the result of a single resource operation
type ResourceResult struct {
	// Name is the resource name from config
	Name string
	// Kind is the Kubernetes resource kind
	Kind string
	// Namespace is the resource namespace
	Namespace string
	// ResourceName is the actual K8s resource name
	ResourceName string
	// Status is the result status
	Status ExecutionStatus
	// Operation is the operation performed (create, update, skip)
	Operation ResourceOperation
	// Resource is the created/updated resource (if successful)
	Resource *unstructured.Unstructured
	// OperationReason explains why this operation was performed
	// Examples: "resource not found", "generation changed from 1 to 2", "generation 1 unchanged", "recreateOnChange=true"
	OperationReason string
	// Error is the error if Status is StatusFailed
	Error error
}

// ResourceOperation represents the operation performed on a resource
type ResourceOperation string

const (
	// OperationCreate indicates a resource was created
	OperationCreate ResourceOperation = "create"
	// OperationUpdate indicates a resource was updated
	OperationUpdate ResourceOperation = "update"
	// OperationRecreate indicates a resource was deleted and recreated
	OperationRecreate ResourceOperation = "recreate"
	// OperationSkip indicates no operation was needed
	OperationSkip ResourceOperation = "skip"
)

// ExecutionContext holds runtime context during execution
type ExecutionContext struct {
	// Ctx is the Go context
	Ctx context.Context
	// EventData is the parsed event data payload
	EventData map[string]interface{}
	// Params holds extracted parameters and captured fields
	Params map[string]interface{}
	// Resources holds created/updated K8s resources keyed by resource name
	Resources map[string]*unstructured.Unstructured
	// Adapter holds adapter execution metadata
	Adapter AdapterMetadata
}

// AdapterMetadata holds adapter execution metadata for CEL expressions
type AdapterMetadata struct {
	// ExecutionStatus is the overall execution status (runtime perspective: "success", "failed")
	ExecutionStatus string
	// ErrorReason is the error reason if failed (process execution errors only)
	ErrorReason string
	// ErrorMessage is the error message if failed (process execution errors only)
	ErrorMessage string
	// ExecutionError contains detailed error information if execution failed
	ExecutionError *ExecutionError `json:"executionError,omitempty"`
}

// ExecutionError represents a structured execution error
type ExecutionError struct {
	// Step is the specific step that failed
	Step string `json:"step"`
	// Message is the error message (includes all relevant details)
	Message string `json:"message"`
}

// NewExecutionContext creates a new execution context
func NewExecutionContext(ctx context.Context, eventData map[string]interface{}) *ExecutionContext {
	return &ExecutionContext{
		Ctx:       ctx,
		EventData: eventData,
		Params:    make(map[string]interface{}),
		Resources: make(map[string]*unstructured.Unstructured),
		Adapter: AdapterMetadata{
			ExecutionStatus: string(StatusSuccess),
		},
	}
}

// SetError sets the error status in adapter metadata (for runtime failures)
func (ec *ExecutionContext) SetError(reason, message string) {
	ec.Adapter.ExecutionStatus = string(StatusFailed)
	ec.Adapter.ErrorReason = reason
	ec.Adapter.ErrorMessage = message
}

// GetCELVariables returns all variables for CEL evaluation.
// This includes Params, adapter metadata, and resources.
func (ec *ExecutionContext) GetCELVariables() map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all params
	for k, v := range ec.Params {
		result[k] = v
	}

	// Add adapter metadata (use helper from utils.go)
	result["adapter"] = adapterMetadataToMap(&ec.Adapter)

	// Add resources (convert unstructured to maps)
	resources := make(map[string]interface{})
	for name, resource := range ec.Resources {
		if resource != nil {
			resources[name] = resource.Object
		}
	}
	result["resources"] = resources

	return result
}

// ExecutorError represents an error during execution
type ExecutorError struct {
	Step    string
	Message string
	Err     error
}

func (e *ExecutorError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Step, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Step, e.Message)
}

func (e *ExecutorError) Unwrap() error {
	return e.Err
}

// NewExecutorError creates a new executor error
func NewExecutorError(step, message string, err error) *ExecutorError {
	return &ExecutorError{
		Step:    step,
		Message: message,
		Err:     err,
	}
}
