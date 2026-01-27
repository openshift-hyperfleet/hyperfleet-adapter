package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

// StepExecutor executes individual steps in the step-based execution model.
// It handles all step types: param, apiCall, resource, payload, and log.
type StepExecutor struct {
	apiClient hyperfleet_api.Client
	k8sClient k8s_client.K8sClient
	log       logger.Logger
}

// NewStepExecutor creates a new step executor
func NewStepExecutor(apiClient hyperfleet_api.Client, k8sClient k8s_client.K8sClient, log logger.Logger) *StepExecutor {
	return &StepExecutor{
		apiClient: apiClient,
		k8sClient: k8sClient,
		log:       log,
	}
}

// ExecuteStep executes a single step and returns the result.
// If the step has a 'when' clause that evaluates to false, the step is skipped.
// Errors are soft failures - the step result contains the error but execution continues.
func (se *StepExecutor) ExecuteStep(ctx context.Context, step config_loader.Step, execCtx *StepExecutionContext) *StepResult {
	stepType := StepType(step.GetStepType())

	// Evaluate 'when' clause if present
	if step.When != "" {
		matched, err := se.evaluateWhen(ctx, step.When, execCtx)
		if err != nil {
			return NewStepResultError(step.Name, stepType, "WhenEvaluationFailed", err.Error())
		}
		if !matched {
			return NewStepResultSkipped(step.Name, stepType, fmt.Sprintf("when clause evaluated to false: %s", step.When))
		}
	}

	// Execute based on step type
	switch {
	case step.Param != nil:
		return se.executeParamStep(ctx, step, execCtx)
	case step.APICall != nil:
		return se.executeAPICallStep(ctx, step, execCtx)
	case step.Resource != nil:
		return se.executeResourceStep(ctx, step, execCtx)
	case step.Payload != nil:
		return se.executePayloadStep(ctx, step, execCtx)
	case step.Log != nil:
		return se.executeLogStep(ctx, step, execCtx)
	default:
		return NewStepResultError(step.Name, "", "UnknownStepType", "step has no recognized type field (param, apiCall, resource, payload, log)")
	}
}

// evaluateWhen evaluates a CEL expression for the 'when' clause
func (se *StepExecutor) evaluateWhen(ctx context.Context, expression string, execCtx *StepExecutionContext) (bool, error) {
	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())

	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, se.log)
	if err != nil {
		return false, fmt.Errorf("failed to create evaluator: %w", err)
	}

	result, err := evaluator.EvaluateCEL(expression)
	if err != nil {
		return false, fmt.Errorf("CEL evaluation failed: %w", err)
	}

	return result.Matched, nil
}

// executeParamStep executes a parameter step
func (se *StepExecutor) executeParamStep(ctx context.Context, step config_loader.Step, execCtx *StepExecutionContext) *StepResult {
	ps := step.Param
	var value interface{}
	var err error

	switch {
	case ps.Value != nil:
		// Literal value
		value = ps.Value

	case ps.Expression != "":
		// CEL expression
		value, err = se.evaluateCELExpression(ctx, ps.Expression, execCtx)
		if err != nil {
			if ps.Default != nil {
				value = ps.Default
				err = nil
			} else {
				return NewStepResultError(step.Name, StepTypeParam, "ExpressionEvaluationFailed", err.Error())
			}
		}

	case ps.Source != "":
		// Extract from source (env.*, event.*)
		value, err = se.extractFromSource(ctx, ps.Source, execCtx)
		if err != nil {
			// Use default if provided, otherwise value remains nil (soft behavior)
			if ps.Default != nil {
				value = ps.Default
			}
			// Clear error - missing sources are not fatal, value is just nil
			err = nil
		}

	default:
		// No source, value, or expression - use default
		value = ps.Default
	}

	// Apply default if value is nil or empty
	if (value == nil || value == "") && ps.Default != nil {
		value = ps.Default
	}

	return NewStepResult(step.Name, StepTypeParam, value)
}

// extractFromSource extracts a value from the specified source
func (se *StepExecutor) extractFromSource(ctx context.Context, source string, execCtx *StepExecutionContext) (interface{}, error) {
	switch {
	case strings.HasPrefix(source, "env."):
		envVar := source[4:]
		value, exists := os.LookupEnv(envVar)
		if !exists {
			return nil, fmt.Errorf("environment variable %s not set", envVar)
		}
		return value, nil

	case strings.HasPrefix(source, "event."):
		path := source[6:]
		return se.extractFromMap(path, execCtx.EventData)

	case strings.HasPrefix(source, "secret."):
		if se.k8sClient == nil {
			return nil, fmt.Errorf("kubernetes client not configured, cannot extract from secret")
		}
		return se.k8sClient.ExtractFromSecret(ctx, source[7:])

	case strings.HasPrefix(source, "configmap."):
		if se.k8sClient == nil {
			return nil, fmt.Errorf("kubernetes client not configured, cannot extract from configmap")
		}
		return se.k8sClient.ExtractFromConfigMap(ctx, source[10:])

	default:
		// Try to extract from event data directly
		return se.extractFromMap(source, execCtx.EventData)
	}
}

// extractFromMap extracts a value from a map using dot notation
func (se *StepExecutor) extractFromMap(path string, data map[string]interface{}) (interface{}, error) {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for i, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("field '%s' not found at path '%s'", part, strings.Join(parts[:i+1], "."))
			}
			current = val
		case map[interface{}]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("field '%s' not found at path '%s'", part, strings.Join(parts[:i+1], "."))
			}
			current = val
		default:
			return nil, fmt.Errorf("cannot access field '%s': parent is not a map (got %T)", part, current)
		}
	}

	return current, nil
}

// evaluateCELExpression evaluates a CEL expression and returns the result value
func (se *StepExecutor) evaluateCELExpression(ctx context.Context, expression string, execCtx *StepExecutionContext) (interface{}, error) {
	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())

	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, se.log)
	if err != nil {
		return nil, fmt.Errorf("failed to create evaluator: %w", err)
	}

	result, err := evaluator.EvaluateCEL(expression)
	if err != nil {
		return nil, fmt.Errorf("CEL evaluation failed: %w", err)
	}

	return result.Value, nil
}

// executeAPICallStep executes an API call step
func (se *StepExecutor) executeAPICallStep(ctx context.Context, step config_loader.Step, execCtx *StepExecutionContext) *StepResult {
	apiCallStep := step.APICall

	// Convert to config_loader.APICall for reuse of existing utility
	apiCall := &config_loader.APICall{
		Method:        apiCallStep.Method,
		URL:           apiCallStep.URL,
		Timeout:       apiCallStep.Timeout,
		RetryAttempts: apiCallStep.RetryAttempts,
		RetryBackoff:  apiCallStep.RetryBackoff,
		Headers:       apiCallStep.Headers,
		Body:          apiCallStep.Body,
	}

	// Create a temporary ExecutionContext for the utility function
	tempExecCtx := &ExecutionContext{
		Ctx:       execCtx.Ctx,
		EventData: execCtx.EventData,
		Params:    execCtx.GetTemplateVariables(),
		Resources: execCtx.Resources,
		Adapter:   execCtx.Adapter,
	}

	// Execute API call
	resp, _, err := ExecuteAPICall(ctx, apiCall, tempExecCtx, se.apiClient, se.log)
	if err != nil {
		return NewStepResultError(step.Name, StepTypeAPICall, "APICallFailed", err.Error())
	}

	// Validate response
	if err := ValidateAPIResponse(resp, nil, apiCall.Method, apiCall.URL); err != nil {
		return NewStepResultError(step.Name, StepTypeAPICall, "APIResponseError", err.Error())
	}

	// Parse response as JSON
	var responseData map[string]interface{}
	if len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, &responseData); err != nil {
			return NewStepResultError(step.Name, StepTypeAPICall, "ResponseParseError", fmt.Sprintf("failed to parse response as JSON: %v", err))
		}
	}

	// Process captures - extract fields to top-level variables
	if len(apiCallStep.Capture) > 0 {
		se.log.Debugf(ctx, "Capturing %d fields from API response", len(apiCallStep.Capture))

		captureCtx := criteria.NewEvaluationContext()
		captureCtx.SetVariablesFromMap(responseData)

		captureEvaluator, evalErr := criteria.NewEvaluator(ctx, captureCtx, se.log)
		if evalErr != nil {
			se.log.Warnf(ctx, "Failed to create capture evaluator: %v", evalErr)
		} else {
			for _, capture := range apiCallStep.Capture {
				extractResult, err := captureEvaluator.ExtractValue(capture.Field, capture.Expression)
				if err != nil {
					se.log.Warnf(ctx, "Failed to extract capture '%s': %v", capture.Name, err)
					continue
				}
				if extractResult.Error != nil {
					se.log.Warnf(ctx, "Capture '%s' extraction error: %v", capture.Name, extractResult.Error)
					continue
				}

				// Promote captured value to top-level variable
				execCtx.SetVariable(capture.Name, extractResult.Value)
				se.log.Debugf(ctx, "Captured %s = %v (from %s)", capture.Name, extractResult.Value, extractResult.Source)
			}
		}
	}

	return NewStepResult(step.Name, StepTypeAPICall, responseData)
}

// executeResourceStep executes a Kubernetes resource step
func (se *StepExecutor) executeResourceStep(ctx context.Context, step config_loader.Step, execCtx *StepExecutionContext) *StepResult {
	resourceStep := step.Resource

	// Convert to config_loader.Resource for reuse of existing logic
	resource := config_loader.Resource{
		Name:             step.Name,
		Manifest:         resourceStep.Manifest,
		Discovery:        resourceStep.Discovery,
		RecreateOnChange: resourceStep.RecreateOnChange,
	}

	// Create resource executor for the operation
	resourceExec := newResourceExecutor(&ExecutorConfig{
		APIClient: se.apiClient,
		K8sClient: se.k8sClient,
		Logger:    se.log,
	})

	// Create temporary ExecutionContext for resource executor
	tempExecCtx := &ExecutionContext{
		Ctx:       execCtx.Ctx,
		EventData: execCtx.EventData,
		Params:    execCtx.GetTemplateVariables(),
		Resources: execCtx.Resources,
		Adapter:   execCtx.Adapter,
	}

	// Execute resource operation using ExecuteAll with a single resource
	results, err := resourceExec.ExecuteAll(ctx, []config_loader.Resource{resource}, tempExecCtx)
	if err != nil {
		return NewStepResultError(step.Name, StepTypeResource, "ResourceOperationFailed", err.Error())
	}

	if len(results) == 0 {
		return NewStepResultError(step.Name, StepTypeResource, "NoResult", "resource operation returned no result")
	}

	result := results[0]
	if result.Status == StatusFailed {
		errMsg := "resource operation failed"
		if result.Error != nil {
			errMsg = result.Error.Error()
		}
		return NewStepResultError(step.Name, StepTypeResource, "ResourceOperationFailed", errMsg)
	}

	// Store resource for later access
	if result.Resource != nil {
		execCtx.SetResource(step.Name, result.Resource)
	}

	// Return the resource object as the result
	var resourceData interface{}
	if result.Resource != nil {
		resourceData = result.Resource.Object
	}

	return NewStepResult(step.Name, StepTypeResource, resourceData)
}

// executePayloadStep executes a payload builder step
func (se *StepExecutor) executePayloadStep(ctx context.Context, step config_loader.Step, execCtx *StepExecutionContext) *StepResult {
	payloadDef := step.Payload

	// Create evaluation context with all CEL variables
	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())

	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, se.log)
	if err != nil {
		return NewStepResultError(step.Name, StepTypePayload, "EvaluatorCreationFailed", err.Error())
	}

	// Build the payload
	params := execCtx.GetTemplateVariables()
	payload, err := se.buildPayload(ctx, payloadDef, evaluator, params)
	if err != nil {
		return NewStepResultError(step.Name, StepTypePayload, "PayloadBuildFailed", err.Error())
	}

	// Convert to JSON string for use in templates
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return NewStepResultError(step.Name, StepTypePayload, "PayloadSerializationFailed", err.Error())
	}

	// Store the JSON string in variables for template access
	// Note: We store the JSON string (not the map) because templates need the serialized form
	// for API request bodies. The step result also uses the JSON string for consistency.
	execCtx.SetVariable(step.Name, string(payloadJSON))

	// Return the payload map as the result for programmatic access (e.g., in tests)
	// Note: SetStepResult will NOT overwrite the JSON string in Variables for payload steps
	return NewStepResult(step.Name, StepTypePayload, payload)
}

// buildPayload builds a payload from a build definition
func (se *StepExecutor) buildPayload(ctx context.Context, build interface{}, evaluator *criteria.Evaluator, params map[string]interface{}) (interface{}, error) {
	switch v := build.(type) {
	case map[string]interface{}:
		return se.buildMapPayload(ctx, v, evaluator, params)
	case map[interface{}]interface{}:
		converted := make(map[string]interface{})
		for key, val := range v {
			if strKey, ok := key.(string); ok {
				converted[strKey] = val
			}
		}
		return se.buildMapPayload(ctx, converted, evaluator, params)
	default:
		return build, nil
	}
}

// buildMapPayload builds a map payload, evaluating expressions as needed
func (se *StepExecutor) buildMapPayload(ctx context.Context, m map[string]interface{}, evaluator *criteria.Evaluator, params map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for k, v := range m {
		// Render the key
		renderedKey, err := renderTemplate(k, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render key '%s': %w", k, err)
		}

		// Process the value
		processedValue, err := se.processPayloadValue(ctx, v, evaluator, params)
		if err != nil {
			return nil, fmt.Errorf("failed to process value for key '%s': %w", k, err)
		}

		result[renderedKey] = processedValue
	}

	return result, nil
}

// processPayloadValue processes a value in a payload, evaluating expressions as needed
func (se *StepExecutor) processPayloadValue(ctx context.Context, v interface{}, evaluator *criteria.Evaluator, params map[string]interface{}) (interface{}, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		// Check if this is a value definition: { field: "...", default: ... } or { expression: "...", default: ... }
		if valueDef, ok := config_loader.ParseValueDef(val); ok {
			result, err := evaluator.ExtractValue(valueDef.Field, valueDef.Expression)
			if err != nil {
				return nil, err
			}
			// If value is nil, use default
			if result.Value == nil {
				return valueDef.Default, nil
			}
			return result.Value, nil
		}

		// Recursively process nested maps
		return se.buildMapPayload(ctx, val, evaluator, params)

	case map[interface{}]interface{}:
		converted := make(map[string]interface{})
		for key, value := range val {
			if strKey, ok := key.(string); ok {
				converted[strKey] = value
			}
		}
		return se.processPayloadValue(ctx, converted, evaluator, params)

	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			processed, err := se.processPayloadValue(ctx, item, evaluator, params)
			if err != nil {
				return nil, err
			}
			result[i] = processed
		}
		return result, nil

	case string:
		return renderTemplate(val, params)

	default:
		return v, nil
	}
}

// executeLogStep executes a logging step
func (se *StepExecutor) executeLogStep(ctx context.Context, step config_loader.Step, execCtx *StepExecutionContext) *StepResult {
	logStep := step.Log

	// Render the message template
	message, err := renderTemplate(logStep.Message, execCtx.GetTemplateVariables())
	if err != nil {
		return NewStepResultError(step.Name, StepTypeLog, "MessageRenderFailed", err.Error())
	}

	// Log at the specified level
	level := strings.ToLower(logStep.Level)
	if level == "" {
		level = "info"
	}

	switch level {
	case "debug":
		se.log.Debugf(ctx, "[step:%s] %s", step.Name, message)
	case "info":
		se.log.Infof(ctx, "[step:%s] %s", step.Name, message)
	case "warning", "warn":
		se.log.Warnf(ctx, "[step:%s] %s", step.Name, message)
	case "error":
		se.log.Errorf(ctx, "[step:%s] %s", step.Name, message)
	default:
		se.log.Infof(ctx, "[step:%s] %s", step.Name, message)
	}

	return NewStepResult(step.Name, StepTypeLog, nil)
}

// ExecuteAll executes all steps sequentially and returns the combined result
func (se *StepExecutor) ExecuteAll(ctx context.Context, steps []config_loader.Step, execCtx *StepExecutionContext) *StepExecutionResult {
	result := NewStepExecutionResult()

	for _, step := range steps {
		se.log.Infof(ctx, "Executing step: %s (type: %s)", step.Name, step.GetStepType())

		stepResult := se.ExecuteStep(ctx, step, execCtx)
		execCtx.SetStepResult(stepResult)
		result.AddStepResult(stepResult)

		if stepResult.Skipped {
			se.log.Infof(ctx, "Step %s: SKIPPED - %s", step.Name, stepResult.SkipReason)
			continue
		}

		if stepResult.Error != nil {
			se.log.Warnf(ctx, "Step %s: ERROR - %s: %s", step.Name, stepResult.Error.Reason, stepResult.Error.Message)
			// Continue execution - errors are soft failures
			continue
		}

		se.log.Infof(ctx, "Step %s: SUCCESS", step.Name)
	}

	// Determine overall status
	if result.HasErrors {
		result.Status = StatusFailed
	}

	// Copy final variables
	result.Variables = execCtx.GetTemplateVariables()

	return result
}
