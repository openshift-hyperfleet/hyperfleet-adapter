package executor

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// StepExecutionContext holds runtime state during step-based execution.
// It tracks all step results, variables, and resources created during execution.
type StepExecutionContext struct {
	// Ctx is the Go context for cancellation and timeouts
	Ctx context.Context

	// EventData is the parsed event data payload
	EventData map[string]interface{}

	// StepResults holds all step results keyed by step name
	StepResults map[string]*StepResult

	// StepOrder tracks the order in which steps were executed
	StepOrder []string

	// Variables holds flattened values for CEL/template access.
	// This includes:
	//   - Step results (by step name)
	//   - Captured fields (promoted to top-level)
	//   - Computed values
	Variables map[string]interface{}

	// Resources holds K8s resources created during execution, keyed by step name
	Resources map[string]*unstructured.Unstructured

	// Adapter holds adapter execution metadata
	Adapter AdapterMetadata

	// Metadata holds adapter config metadata (name, namespace, labels)
	Metadata map[string]interface{}
}

// NewStepExecutionContext creates a new step execution context
func NewStepExecutionContext(ctx context.Context, eventData map[string]interface{}, metadata map[string]interface{}) *StepExecutionContext {
	return &StepExecutionContext{
		Ctx:         ctx,
		EventData:   eventData,
		StepResults: make(map[string]*StepResult),
		StepOrder:   make([]string, 0),
		Variables:   make(map[string]interface{}),
		Resources:   make(map[string]*unstructured.Unstructured),
		Adapter: AdapterMetadata{
			ExecutionStatus: string(StatusSuccess),
		},
		Metadata: metadata,
	}
}

// SetStepResult stores a step result and updates the variables map.
// If the step completed successfully (not skipped, no error), the result
// value is made directly accessible by step name in the variables map.
// Note: Even nil values are added to Variables so they can be referenced
// in templates (will render as empty string or be checkable in CEL).
//
// Exception: API call step results are NOT added to Variables because:
// 1. Important fields should be captured via 'capture' (promoted to top-level)
// 2. This allows CEL expressions like 'stepName.error != null' to work
//    consistently (otherwise the API response would shadow the .error access)
func (ctx *StepExecutionContext) SetStepResult(result *StepResult) {
	ctx.StepResults[result.Name] = result
	ctx.StepOrder = append(ctx.StepOrder, result.Name)

	// Make the result directly accessible by step name for convenience
	// This allows expressions like: clusterPhase == "Ready"
	// instead of: clusterPhase.result == "Ready"
	// Note: nil values are also added so they can be referenced (renders as empty)
	//
	// Skip API call results - they use captures for field extraction, and skipping
	// allows consistent .error access via ToMap() in GetCELVariables
	//
	// Skip payload results - executePayloadStep already sets the variable to JSON string
	// (not the map), and we don't want to overwrite it with the map here
	if result.IsSuccess() && result.Type != StepTypeAPICall && result.Type != StepTypePayload {
		ctx.Variables[result.Name] = result.Result
	}
}

// SetVariable sets a variable in the variables map.
// Used for captured fields that are promoted to top-level.
func (ctx *StepExecutionContext) SetVariable(name string, value interface{}) {
	ctx.Variables[name] = value
}

// GetVariable returns a variable from the variables map
func (ctx *StepExecutionContext) GetVariable(name string) (interface{}, bool) {
	v, ok := ctx.Variables[name]
	return v, ok
}

// SetResource stores a K8s resource in the resources map
func (ctx *StepExecutionContext) SetResource(name string, resource *unstructured.Unstructured) {
	ctx.Resources[name] = resource
}

// GetStepResult returns a step result by name
func (ctx *StepExecutionContext) GetStepResult(name string) *StepResult {
	return ctx.StepResults[name]
}

// GetCELVariables returns all variables available for CEL evaluation.
// This includes:
//   - All variables (step results, captured fields, etc.)
//   - Step results with .error and .skipped accessible
//   - Adapter metadata
//   - Resources
//   - Metadata (adapter config metadata)
func (ctx *StepExecutionContext) GetCELVariables() map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all variables (includes step results and captured fields)
	// This preserves the direct value access: status == "active"
	for k, v := range ctx.Variables {
		result[k] = v
	}

	// For skipped or errored steps that aren't in Variables, add their map structure
	// This allows checking: stepName.error != null, stepName.skipped == true
	for name, stepResult := range ctx.StepResults {
		if _, exists := result[name]; !exists {
			// Step was skipped or errored (not in Variables), add the map structure
			result[name] = stepResult.ToMap()
		}
	}

	// Add adapter metadata
	result["adapter"] = adapterMetadataToMap(&ctx.Adapter)

	// Add resources (convert unstructured to maps)
	resources := make(map[string]interface{})
	for name, resource := range ctx.Resources {
		if resource != nil {
			resources[name] = resource.Object
		}
	}
	result["resources"] = resources

	// Add metadata
	result["metadata"] = ctx.Metadata

	return result
}

// GetTemplateVariables returns all variables for Go template rendering.
// Similar to GetCELVariables but optimized for template usage.
func (ctx *StepExecutionContext) GetTemplateVariables() map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all variables
	for k, v := range ctx.Variables {
		result[k] = v
	}

	// Add metadata
	result["metadata"] = ctx.Metadata

	// Add resources as maps for template access
	for name, resource := range ctx.Resources {
		if resource != nil {
			result[name] = resource.Object
		}
	}

	return result
}

// SetError sets the error status in adapter metadata
func (ctx *StepExecutionContext) SetError(reason, message string) {
	ctx.Adapter.ExecutionStatus = string(StatusFailed)
	ctx.Adapter.ErrorReason = reason
	ctx.Adapter.ErrorMessage = message
}

// HasAnyError returns true if any step had an error
func (ctx *StepExecutionContext) HasAnyError() bool {
	for _, result := range ctx.StepResults {
		if result.Error != nil {
			return true
		}
	}
	return false
}

// GetFirstError returns the first step error that occurred
func (ctx *StepExecutionContext) GetFirstError() *StepError {
	for _, name := range ctx.StepOrder {
		if result := ctx.StepResults[name]; result != nil && result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// stepResultErrorToMap converts a StepError to a map for CEL evaluation
func stepResultErrorToMap(err *StepError) interface{} {
	if err == nil {
		return nil
	}
	return map[string]interface{}{
		"reason":  err.Reason,
		"message": err.Message,
	}
}
