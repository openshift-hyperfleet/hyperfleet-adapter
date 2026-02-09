package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
)

// PostActionsPhase handles post-action execution (API calls, logging, etc.)
type PostActionsPhase struct {
	apiClient hyperfleet_api.Client
	config    *config_loader.Config
	log       logger.Logger
	// results stores the post-action results for later retrieval
	results []PostActionResult
}

// NewPostActionsPhase creates a new post-actions phase
func NewPostActionsPhase(apiClient hyperfleet_api.Client, config *config_loader.Config, log logger.Logger) *PostActionsPhase {
	return &PostActionsPhase{
		apiClient: apiClient,
		config:    config,
		log:       log,
	}
}

// Name returns the phase identifier
func (p *PostActionsPhase) Name() ExecutionPhase {
	return PhasePostActions
}

// ShouldSkip determines if this phase should be skipped
func (p *PostActionsPhase) ShouldSkip(execCtx *ExecutionContext) (bool, string) {
	// Post-actions are always executed (even on error) for error reporting
	// Only skip if there are no post-actions configured
	if p.config.Spec.Post == nil || len(p.config.Spec.Post.PostActions) == 0 {
		return true, "no post-actions configured"
	}
	return false, ""
}

// Execute runs post-action logic
func (p *PostActionsPhase) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	postActionCount := 0
	if p.config.Spec.Post != nil {
		postActionCount = len(p.config.Spec.Post.PostActions)
	}
	p.log.Infof(ctx, "Executing %d post-actions", postActionCount)

	results, err := p.executeAll(ctx, p.config.Spec.Post, execCtx)
	p.results = results

	if err != nil {
		return fmt.Errorf("post action execution failed: %w", err)
	}

	p.log.Infof(ctx, "Successfully executed %d post-actions", len(results))
	return nil
}

// Results returns the post-action results
func (p *PostActionsPhase) Results() []PostActionResult {
	return p.results
}

// executeAll executes all post-processing actions
// First builds payloads from post.payloads, then executes post.postActions
func (p *PostActionsPhase) executeAll(ctx context.Context, postConfig *config_loader.PostConfig, execCtx *ExecutionContext) ([]PostActionResult, error) {
	if postConfig == nil {
		return []PostActionResult{}, nil
	}

	// Step 1: Build post payloads (like clusterStatusPayload)
	if len(postConfig.Payloads) > 0 {
		p.log.Infof(ctx, "Building %d post payloads", len(postConfig.Payloads))
		if err := p.buildPostPayloads(ctx, postConfig.Payloads, execCtx); err != nil {
			errCtx := logger.WithErrorField(ctx, err)
			p.log.Errorf(errCtx, "Failed to build post payloads")
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhasePostActions),
				Step:    "build_payloads",
				Message: err.Error(),
			}
			return []PostActionResult{}, NewExecutorError(PhasePostActions, "build_payloads", "failed to build post payloads", err)
		}
		for _, payload := range postConfig.Payloads {
			p.log.Debugf(ctx, "payload[%s] built successfully", payload.Name)
		}
	}

	// Step 2: Execute post actions (sequential - stop on first failure)
	results := make([]PostActionResult, 0, len(postConfig.PostActions))
	for _, action := range postConfig.PostActions {
		result, err := p.executePostAction(ctx, action, execCtx)
		results = append(results, result)

		if err != nil {
			errCtx := logger.WithErrorField(ctx, err)
			p.log.Errorf(errCtx, "PostAction[%s] processed: FAILED", action.Name)

			// Set ExecutionError for failed post action
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhasePostActions),
				Step:    action.Name,
				Message: err.Error(),
			}

			// Stop execution - don't run remaining post actions
			return results, err
		}
		p.log.Infof(ctx, "PostAction[%s] processed: SUCCESS - status=%s", action.Name, result.Status)
	}

	return results, nil
}

// buildPostPayloads builds all post payloads and stores them in execCtx.Params
// Payloads are complex structures built from CEL expressions and templates
func (p *PostActionsPhase) buildPostPayloads(ctx context.Context, payloads []config_loader.Payload, execCtx *ExecutionContext) error {
	// Create evaluation context with all CEL variables (params, adapter, resources)
	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())

	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, p.log)
	if err != nil {
		return fmt.Errorf("failed to create evaluator: %w", err)
	}

	for _, payload := range payloads {
		// Determine build source (inline Build or BuildRef)
		var buildDef any
		if payload.Build != nil {
			buildDef = payload.Build
		} else if payload.BuildRefContent != nil {
			buildDef = payload.BuildRefContent
		} else {
			return fmt.Errorf("payload '%s' has neither Build nor BuildRefContent", payload.Name)
		}

		// Build the payload
		builtPayload, err := p.buildPayload(ctx, buildDef, evaluator, execCtx.Params)
		if err != nil {
			return fmt.Errorf("failed to build payload '%s': %w", payload.Name, err)
		}

		// Convert to JSON for template rendering (templates will render maps as "map[...]" otherwise)
		jsonBytes, err := json.Marshal(builtPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload '%s' to JSON: %w", payload.Name, err)
		}

		// Store as JSON string in params for use in post action templates
		execCtx.Params[payload.Name] = string(jsonBytes)
	}

	return nil
}

// buildPayload builds a payload from a build definition
// The build definition can contain expressions that need to be evaluated
func (p *PostActionsPhase) buildPayload(ctx context.Context, build any, evaluator *criteria.Evaluator, params map[string]any) (any, error) {
	switch v := build.(type) {
	case map[string]any:
		return p.buildMapPayload(ctx, v, evaluator, params)
	case map[any]any:
		converted := utils.ConvertToStringKeyMap(v)
		return p.buildMapPayload(ctx, converted, evaluator, params)
	default:
		return build, nil
	}
}

// buildMapPayload builds a map payload, evaluating expressions as needed
func (p *PostActionsPhase) buildMapPayload(ctx context.Context, m map[string]any, evaluator *criteria.Evaluator, params map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for k, v := range m {
		// Render the key
		renderedKey, err := renderTemplate(k, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render key '%s': %w", k, err)
		}

		// Process the value
		processedValue, err := p.processValue(ctx, v, evaluator, params)
		if err != nil {
			return nil, fmt.Errorf("failed to process value for key '%s': %w", k, err)
		}

		result[renderedKey] = processedValue
	}

	return result, nil
}

// processValue processes a value, evaluating expressions as needed
func (p *PostActionsPhase) processValue(ctx context.Context, v any, evaluator *criteria.Evaluator, params map[string]any) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		// Check if this is a value definition: { field: "...", default: ... } or { expression: "...", default: ... }
		if valueDef, ok := config_loader.ParseValueDef(val); ok {
			result, err := evaluator.ExtractValue(valueDef.Field, valueDef.Expression)
			// err indicates parse error - fail fast (bug in config)
			if err != nil {
				return nil, err
			}
			// If value is nil (field not found or empty), use default
			if result.Value == nil {
				if valueDef.Default != nil {
					p.log.Debugf(ctx, "Using default value for '%s': %v", result.Source, valueDef.Default)
				}
				return valueDef.Default, nil
			}
			return result.Value, nil
		}

		// Recursively process nested maps
		return p.buildMapPayload(ctx, val, evaluator, params)

	case map[any]any:
		converted := utils.ConvertToStringKeyMap(val)
		return p.processValue(ctx, converted, evaluator, params)

	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			processed, err := p.processValue(ctx, item, evaluator, params)
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

// executePostAction executes a single post-action
func (p *PostActionsPhase) executePostAction(ctx context.Context, action config_loader.PostAction, execCtx *ExecutionContext) (PostActionResult, error) {
	result := PostActionResult{
		Name:   action.Name,
		Status: StatusSuccess,
	}

	// Execute log action if configured
	if action.Log != nil {
		ExecuteLogAction(ctx, action.Log, execCtx, p.log)
	}

	// Execute API call if configured
	if action.APICall != nil {
		if err := p.executeAPICall(ctx, action.APICall, execCtx, &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// executeAPICall executes an API call and populates the result with response details
func (p *PostActionsPhase) executeAPICall(ctx context.Context, apiCall *config_loader.APICall, execCtx *ExecutionContext, result *PostActionResult) error {
	resp, url, err := ExecuteAPICall(ctx, apiCall, execCtx, p.apiClient, p.log)
	result.APICallMade = true

	// Capture response details if available (even if err != nil)
	if resp != nil {
		result.APIResponse = resp.Body
		result.HTTPStatus = resp.StatusCode
	}

	// Validate response - returns APIError with full metadata if validation fails
	if validationErr := ValidateAPIResponse(resp, err, apiCall.Method, url); validationErr != nil {
		result.Status = StatusFailed
		result.Error = validationErr

		// Determine error context
		errorContext := "API call failed"
		if err == nil && resp != nil && !resp.IsSuccess() {
			errorContext = "API call returned non-success status"
		}

		return NewExecutorError(PhasePostActions, result.Name, errorContext, validationErr)
	}

	return nil
}
