package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleet_api"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

// PreconditionsPhase handles precondition evaluation
type PreconditionsPhase struct {
	apiClient hyperfleet_api.Client
	config    *config_loader.Config
	log       logger.Logger
	// results stores the precondition results for later retrieval
	results []PreconditionResult
	// outcome stores the overall outcome for later retrieval
	outcome *PreconditionsOutcome
}

// NewPreconditionsPhase creates a new preconditions phase
func NewPreconditionsPhase(apiClient hyperfleet_api.Client, config *config_loader.Config, log logger.Logger) *PreconditionsPhase {
	return &PreconditionsPhase{
		apiClient: apiClient,
		config:    config,
		log:       log,
	}
}

// Name returns the phase identifier
func (p *PreconditionsPhase) Name() ExecutionPhase {
	return PhasePreconditions
}

// ShouldSkip determines if this phase should be skipped
func (p *PreconditionsPhase) ShouldSkip(execCtx *ExecutionContext) (bool, string) {
	if len(p.config.Spec.Preconditions) == 0 {
		return true, "no preconditions configured"
	}
	return false, ""
}

// Execute runs precondition evaluation logic
func (p *PreconditionsPhase) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	p.log.Infof(ctx, "Evaluating %d preconditions", len(p.config.Spec.Preconditions))

	outcome := p.executeAll(ctx, p.config.Spec.Preconditions, execCtx)
	p.results = outcome.Results
	p.outcome = &PreconditionsOutcome{
		AllMatched:   outcome.AllMatched,
		Results:      outcome.Results,
		Error:        outcome.Error,
		NotMetReason: outcome.NotMetReason,
	}

	if outcome.Error != nil {
		// Process execution error: precondition evaluation failed
		execCtx.SetError("PreconditionFailed", outcome.Error.Error())
		execCtx.SetSkipped("PreconditionFailed", outcome.Error.Error())
		return fmt.Errorf("precondition evaluation failed: %w", outcome.Error)
	}

	if !outcome.AllMatched {
		// Business outcome: precondition not satisfied (not an error)
		execCtx.SetSkipped("PreconditionNotMet", outcome.NotMetReason)
		p.log.Infof(ctx, "Preconditions not met: %s", outcome.NotMetReason)
	} else {
		p.log.Infof(ctx, "All %d preconditions passed", len(outcome.Results))
	}

	return nil
}

// Results returns the precondition evaluation results
func (p *PreconditionsPhase) Results() []PreconditionResult {
	return p.results
}

// Outcome returns the overall preconditions outcome
func (p *PreconditionsPhase) Outcome() *PreconditionsOutcome {
	return p.outcome
}

// executeAll executes all preconditions in sequence
// Returns a high-level outcome with match status and individual results
func (p *PreconditionsPhase) executeAll(ctx context.Context, preconditions []config_loader.Precondition, execCtx *ExecutionContext) *PreconditionsOutcome {
	results := make([]PreconditionResult, 0, len(preconditions))

	for _, precond := range preconditions {
		result, err := p.executePrecondition(ctx, precond, execCtx)
		results = append(results, result)

		if err != nil {
			// Execution error (API call failed, parse error, etc.)
			errCtx := logger.WithErrorField(ctx, err)
			p.log.Errorf(errCtx, "Precondition[%s] evaluated: FAILED", precond.Name)
			return &PreconditionsOutcome{
				AllMatched: false,
				Results:    results,
				Error:      err,
			}
		}

		if !result.Matched {
			// Business outcome: precondition not satisfied
			p.log.Infof(ctx, "Precondition[%s] evaluated: NOT_MET - %s", precond.Name, formatConditionDetails(result))
			return &PreconditionsOutcome{
				AllMatched:   false,
				Results:      results,
				Error:        nil,
				NotMetReason: fmt.Sprintf("precondition '%s' not met: %s", precond.Name, formatConditionDetails(result)),
			}
		}

		p.log.Infof(ctx, "Precondition[%s] evaluated: MET", precond.Name)
	}

	// All preconditions matched
	return &PreconditionsOutcome{
		AllMatched: true,
		Results:    results,
		Error:      nil,
	}
}

// executePrecondition executes a single precondition
func (p *PreconditionsPhase) executePrecondition(ctx context.Context, precond config_loader.Precondition, execCtx *ExecutionContext) (PreconditionResult, error) {
	result := PreconditionResult{
		Name:           precond.Name,
		Status:         StatusSuccess,
		CapturedFields: make(map[string]interface{}),
	}

	// Step 1: Execute log action if configured
	if precond.Log != nil {
		ExecuteLogAction(ctx, precond.Log, execCtx, p.log)
	}

	// Step 2: Make API call if configured
	if precond.APICall != nil {
		apiResult, err := p.executeAPICall(ctx, precond.APICall, execCtx)
		if err != nil {
			result.Status = StatusFailed
			result.Error = err

			// Set ExecutionError for API call failure
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhasePreconditions),
				Step:    precond.Name,
				Message: err.Error(),
			}

			return result, NewExecutorError(PhasePreconditions, precond.Name, "API call failed", err)
		}
		result.APICallMade = true
		result.APIResponse = apiResult

		// Parse response as JSON
		var responseData map[string]interface{}
		if err := json.Unmarshal(apiResult, &responseData); err != nil {
			result.Status = StatusFailed
			result.Error = fmt.Errorf("failed to parse API response as JSON: %w", err)

			// Set ExecutionError for parse failure
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhasePreconditions),
				Step:    precond.Name,
				Message: err.Error(),
			}

			return result, NewExecutorError(PhasePreconditions, precond.Name, "failed to parse API response", err)
		}

		// Store full response under precondition name for condition digging
		// e.g., conditions can access "check-cluster.status.conditions"
		execCtx.Params[precond.Name] = responseData

		// Capture fields from response
		if len(precond.Capture) > 0 {
			p.log.Debugf(ctx, "Capturing %d fields from API response", len(precond.Capture))

			// Create evaluator with response data only
			// Both field (JSONPath) and expression (CEL) work on the same source
			captureCtx := criteria.NewEvaluationContext()
			captureCtx.SetVariablesFromMap(responseData)

			captureEvaluator, evalErr := criteria.NewEvaluator(ctx, captureCtx, p.log)
			if evalErr != nil {
				p.log.Warnf(ctx, "Failed to create capture evaluator: %v", evalErr)
			} else {
				for _, capture := range precond.Capture {
					extractResult, err := captureEvaluator.ExtractValue(capture.Field, capture.Expression)
					if err != nil {
						return result, err
					}
					// Error is not nil when there is field missing that is not a bug, but a valid use case
					if extractResult.Error != nil {
						p.log.Warnf(ctx, "Failed to capture '%s' with error: %v", capture.Name, extractResult.Error)
						continue
					}
					result.CapturedFields[capture.Name] = extractResult.Value
					execCtx.Params[capture.Name] = extractResult.Value
					p.log.Debugf(ctx, "Captured %s = %v (from %s)", capture.Name, extractResult.Value, extractResult.Source)
				}
			}
		}
	}

	// Step 3: Evaluate conditions
	// Create evaluation context with all CEL variables (params, adapter, resources)
	// Note: resources will be empty during preconditions since they haven't been created yet
	evalCtx := criteria.NewEvaluationContext()
	evalCtx.SetVariablesFromMap(execCtx.GetCELVariables())

	evaluator, err := criteria.NewEvaluator(ctx, evalCtx, p.log)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		return result, NewExecutorError(PhasePreconditions, precond.Name, "failed to create evaluator", err)
	}

	// Evaluate using structured conditions or CEL expression
	if len(precond.Conditions) > 0 {
		p.log.Debugf(ctx, "Evaluating %d structured conditions", len(precond.Conditions))
		condDefs := ToConditionDefs(precond.Conditions)

		condResult, err := evaluator.EvaluateConditions(condDefs)
		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			return result, NewExecutorError(PhasePreconditions, precond.Name, "condition evaluation failed", err)
		}

		result.Matched = condResult.Matched
		result.ConditionResults = condResult.Results

		// Log individual condition results
		for _, cr := range condResult.Results {
			if cr.Matched {
				p.log.Debugf(ctx, "Condition: %s %s %v = %v (matched)", cr.Field, cr.Operator, cr.ExpectedValue, cr.FieldValue)
			} else {
				p.log.Debugf(ctx, "Condition: %s %s %v = %v (not matched)", cr.Field, cr.Operator, cr.ExpectedValue, cr.FieldValue)
			}
		}

		// Record evaluation in execution context - reuse criteria.EvaluationResult directly
		fieldResults := make(map[string]criteria.EvaluationResult, len(condResult.Results))
		for _, cr := range condResult.Results {
			fieldResults[cr.Field] = cr
		}
		execCtx.AddConditionsEvaluation(PhasePreconditions, precond.Name, condResult.Matched, fieldResults)
	} else if precond.Expression != "" {
		// Evaluate CEL expression
		p.log.Debugf(ctx, "Evaluating CEL expression: %s", strings.TrimSpace(precond.Expression))
		celResult, err := evaluator.EvaluateCEL(strings.TrimSpace(precond.Expression))
		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			return result, NewExecutorError(PhasePreconditions, precond.Name, "CEL expression evaluation failed", err)
		}

		result.Matched = celResult.Matched
		result.CELResult = celResult
		p.log.Debugf(ctx, "CEL result: matched=%v value=%v", celResult.Matched, celResult.Value)

		// Record CEL evaluation in execution context
		execCtx.AddCELEvaluation(PhasePreconditions, precond.Name, precond.Expression, celResult.Matched)
	} else {
		// No conditions specified - consider it matched
		p.log.Debugf(ctx, "No conditions specified, auto-matched")
		result.Matched = true
	}

	return result, nil
}

// executeAPICall executes an API call and returns the response body for field capture
func (p *PreconditionsPhase) executeAPICall(ctx context.Context, apiCall *config_loader.APICall, execCtx *ExecutionContext) ([]byte, error) {
	resp, url, err := ExecuteAPICall(ctx, apiCall, execCtx, p.apiClient, p.log)

	// Validate response - returns APIError with full metadata if validation fails
	if validationErr := ValidateAPIResponse(resp, err, apiCall.Method, url); validationErr != nil {
		return nil, validationErr
	}

	return resp.Body, nil
}

// formatConditionDetails formats condition evaluation details for error messages
func formatConditionDetails(result PreconditionResult) string {
	var details []string

	if result.CELResult != nil && result.CELResult.HasError() {
		details = append(details, fmt.Sprintf("CEL error: %v", result.CELResult.Error))
	}

	for _, condResult := range result.ConditionResults {
		if !condResult.Matched {
			details = append(details, fmt.Sprintf("%s %s %v (actual: %v)",
				condResult.Field, condResult.Operator, condResult.ExpectedValue, condResult.FieldValue))
		}
	}

	if len(details) == 0 {
		return "no specific details available"
	}

	return strings.Join(details, "; ")
}
