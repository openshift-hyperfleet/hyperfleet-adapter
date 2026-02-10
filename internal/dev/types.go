package dev

import (
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
)

// ValidationResult contains the result of config validation
type ValidationResult struct {
	// Valid indicates overall validation success
	Valid bool
	// Errors contains validation errors
	Errors []ValidationIssue
	// Warnings contains validation warnings
	Warnings []ValidationIssue
	// Details contains component-specific validation results
	Details *ValidationDetails
}

// ValidationIssue represents a single validation error or warning
type ValidationIssue struct {
	// Path is the location in the config (e.g., "spec.preconditions[0].expression")
	Path string
	// Message describes the issue
	Message string
	// Type indicates the kind of validation (schema, cel, template, manifest)
	Type string
}

// ValidationDetails contains detailed validation results by category
type ValidationDetails struct {
	Schema    ValidationCategory
	Params    ValidationCategory
	CEL       ValidationCategory
	Templates ValidationCategory
	Manifests ValidationCategory
}

// ValidationCategory represents validation results for a specific category
type ValidationCategory struct {
	Passed bool
	Count  int
	Issues []ValidationIssue
}

// TraceResult contains detailed execution trace for dry-run mode
type TraceResult struct {
	// Success indicates if the overall execution succeeded
	Success bool
	// ExecutionResult is the underlying executor result
	ExecutionResult *executor.ExecutionResult
	// Phases contains trace information for each phase
	Phases []PhaseTrace
	// Duration is the total execution duration
	Duration time.Duration
	// RenderedOutputs contains rendered templates and manifests
	RenderedOutputs *RenderedOutputs
}

// PhaseTrace contains trace information for a single execution phase
type PhaseTrace struct {
	// Phase is the execution phase
	Phase executor.ExecutionPhase
	// Status indicates success or failure
	Status string
	// Duration is how long the phase took
	Duration time.Duration
	// Details contains phase-specific details
	Details map[string]interface{}
	// Error contains any error that occurred
	Error error
}

// RenderedOutputs contains rendered templates and manifests for preview
type RenderedOutputs struct {
	// Manifests maps resource name to rendered YAML
	Manifests map[string]string
	// Payloads maps payload name to rendered JSON
	Payloads map[string]string
	// APICalls contains simulated API call details
	APICalls []APICallTrace
}

// APICallTrace represents a traced API call
type APICallTrace struct {
	// Method is the HTTP method
	Method string
	// URL is the target URL
	URL string
	// Body is the request body (if any)
	Body string
	// Response is the mock response (if configured)
	Response string
	// StatusCode is the mock status code
	StatusCode int
}

// ResourceTrace represents a traced resource operation
type ResourceTrace struct {
	// Name is the resource config name
	Name string
	// Kind is the Kubernetes kind
	Kind string
	// APIVersion is the Kubernetes apiVersion
	APIVersion string
	// Namespace is the target namespace
	Namespace string
	// ResourceName is the Kubernetes resource name
	ResourceName string
	// Operation is what would happen (create, update, skip)
	Operation string
	// Manifest is the rendered manifest YAML
	Manifest string
}
