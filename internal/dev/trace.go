package dev

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
)

// ExecuteWithTrace runs the adapter executor in dry-run mode and captures a detailed trace
func ExecuteWithTrace(ctx context.Context, configPath string, eventData map[string]interface{}, opts *RunOptions) (*TraceResult, error) {
	startTime := time.Now()

	// Load the configuration
	config, err := config_loader.Load(configPath)
	if err != nil {
		return nil, err
	}

	// Create dry-run clients
	k8sClient := NewDryRunK8sClient()
	apiClient := NewMockAPIClient()

	// Load mock API responses if provided
	if opts != nil && opts.MockAPIResponsesPath != "" {
		if err := apiClient.LoadResponsesFromFile(opts.MockAPIResponsesPath); err != nil {
			return nil, err
		}
	}

	// Create a test logger for dry-run (minimal output)
	log := logger.NewTestLogger()

	// Build the executor
	exec, err := executor.NewBuilder().
		WithAdapterConfig(config).
		WithAPIClient(apiClient).
		WithK8sClient(k8sClient).
		WithLogger(log).
		Build()
	if err != nil {
		return nil, err
	}

	// Convert event data to JSON for the executor
	eventBytes, err := json.Marshal(eventData)
	if err != nil {
		return nil, err
	}

	// Execute
	execResult := exec.Execute(ctx, eventBytes)

	// Build the trace result
	trace := &TraceResult{
		Success:         execResult.Status == executor.StatusSuccess,
		ExecutionResult: execResult,
		Duration:        time.Since(startTime),
		Phases:          buildPhaseTraces(execResult),
		RenderedOutputs: &RenderedOutputs{
			Manifests: k8sClient.GetManifests(),
			Payloads:  make(map[string]string),
			APICalls:  buildAPICallTraces(apiClient.GetCallRecords()),
		},
	}

	return trace, nil
}

// RunOptions configures the dry-run execution
type RunOptions struct {
	// MockAPIResponsesPath is the path to mock API responses file
	MockAPIResponsesPath string
	// ShowManifests enables manifest output
	ShowManifests bool
	// ShowPayloads enables payload output
	ShowPayloads bool
	// ShowParams enables parameter output
	ShowParams bool
	// EnvFilePath is the path to .env file
	EnvFilePath string
}

// buildPhaseTraces builds phase traces from execution result
func buildPhaseTraces(result *executor.ExecutionResult) []PhaseTrace {
	phases := []PhaseTrace{}

	// Phase 1: Parameter Extraction
	paramDetails := make(map[string]interface{})
	if result.Params != nil {
		paramDetails["count"] = len(result.Params)
		paramDetails["params"] = result.Params
	}
	phases = append(phases, PhaseTrace{
		Phase:   executor.PhaseParamExtraction,
		Status:  getPhaseStatus(executor.PhaseParamExtraction, result),
		Details: paramDetails,
		Error:   result.Errors[executor.PhaseParamExtraction],
	})

	// Phase 2: Preconditions
	precondDetails := make(map[string]interface{})
	if result.PreconditionResults != nil {
		precondDetails["count"] = len(result.PreconditionResults)
		matched := 0
		for _, r := range result.PreconditionResults {
			if r.Matched {
				matched++
			}
		}
		precondDetails["matched"] = matched
	}
	phases = append(phases, PhaseTrace{
		Phase:   executor.PhasePreconditions,
		Status:  getPhaseStatus(executor.PhasePreconditions, result),
		Details: precondDetails,
		Error:   result.Errors[executor.PhasePreconditions],
	})

	// Phase 3: Resources
	resourceDetails := make(map[string]interface{})
	if result.ResourceResults != nil {
		resourceDetails["count"] = len(result.ResourceResults)
		operations := make(map[string]int)
		for _, r := range result.ResourceResults {
			operations[string(r.Operation)]++
		}
		resourceDetails["operations"] = operations
	}
	if result.ResourcesSkipped {
		resourceDetails["skipped"] = true
		resourceDetails["skipReason"] = result.SkipReason
	}
	phases = append(phases, PhaseTrace{
		Phase:   executor.PhaseResources,
		Status:  getResourcePhaseStatus(result),
		Details: resourceDetails,
		Error:   result.Errors[executor.PhaseResources],
	})

	// Phase 4: Post Actions
	postDetails := make(map[string]interface{})
	if result.PostActionResults != nil {
		postDetails["count"] = len(result.PostActionResults)
		executed := 0
		for _, r := range result.PostActionResults {
			if r.APICallMade {
				executed++
			}
		}
		postDetails["executed"] = executed
	}
	phases = append(phases, PhaseTrace{
		Phase:   executor.PhasePostActions,
		Status:  getPhaseStatus(executor.PhasePostActions, result),
		Details: postDetails,
		Error:   result.Errors[executor.PhasePostActions],
	})

	return phases
}

// getPhaseStatus determines the status string for a phase
func getPhaseStatus(phase executor.ExecutionPhase, result *executor.ExecutionResult) string {
	if result.Errors[phase] != nil {
		return "failed"
	}
	return "success"
}

// getResourcePhaseStatus determines the status string for the resource phase
func getResourcePhaseStatus(result *executor.ExecutionResult) string {
	if result.Errors[executor.PhaseResources] != nil {
		return "failed"
	}
	if result.ResourcesSkipped {
		return "skipped"
	}
	return "dry-run"
}

// buildAPICallTraces builds API call traces from recorded API calls
func buildAPICallTraces(records []APICallRecord) []APICallTrace {
	traces := make([]APICallTrace, len(records))
	for i, record := range records {
		trace := APICallTrace{
			Method:     record.Request.Method,
			URL:        record.Request.URL,
			Body:       string(record.Request.Body),
			StatusCode: 200, // Default status code
		}

		// Include response data if available
		if record.Response != nil {
			trace.StatusCode = record.Response.StatusCode
			if record.Response.Body != nil {
				if bodyBytes, err := json.Marshal(record.Response.Body); err == nil {
					trace.Response = string(bodyBytes)
				}
			}
		}

		traces[i] = trace
	}
	return traces
}
