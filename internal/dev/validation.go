package dev

import (
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
)

// ValidateConfig performs detailed validation of an adapter configuration file
func ValidateConfig(configPath string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []ValidationIssue{},
		Warnings: []ValidationIssue{},
		Details: &ValidationDetails{
			Schema:    ValidationCategory{Passed: true},
			Params:    ValidationCategory{Passed: true},
			CEL:       ValidationCategory{Passed: true},
			Templates: ValidationCategory{Passed: true},
			Manifests: ValidationCategory{Passed: true},
		},
	}

	// Load and validate the configuration
	config, err := config_loader.Load(configPath)
	if err != nil {
		// Parse the error to categorize it
		categorizeLoadError(err, result)
		result.Valid = false
		// Add the raw error to Errors for visibility
		if len(result.Errors) == 0 {
			result.Errors = append(result.Errors, ValidationIssue{
				Path:    "config",
				Message: err.Error(),
				Type:    "schema",
			})
		}
		return result, nil
	}

	// Config loaded successfully - validation passed
	result.Valid = true

	// Count items for details
	countConfigItems(config, result)

	return result, nil
}

// categorizeLoadError parses validation errors and categorizes them
func categorizeLoadError(err error, result *ValidationResult) {
	errStr := err.Error()

	// Check if it's a validation errors collection
	if strings.Contains(errStr, "validation failed") {
		// Parse individual errors from the ValidationErrors
		lines := strings.Split(errStr, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "validation failed") {
				continue
			}
			line = strings.TrimPrefix(line, "- ")

			issue := parseValidationLine(line)
			result.Errors = append(result.Errors, issue)
			categorizeIssue(issue, result)
		}
	} else {
		// Single error - likely a file or parsing error
		issue := ValidationIssue{
			Path:    "config",
			Message: errStr,
			Type:    "schema",
		}
		result.Errors = append(result.Errors, issue)
		result.Details.Schema.Passed = false
		result.Details.Schema.Issues = append(result.Details.Schema.Issues, issue)
	}
}

// parseValidationLine parses a validation error line into a ValidationIssue
func parseValidationLine(line string) ValidationIssue {
	issue := ValidationIssue{
		Type: "unknown",
	}

	// Format is typically "path: message"
	parts := strings.SplitN(line, ": ", 2)
	if len(parts) == 2 {
		issue.Path = parts[0]
		issue.Message = parts[1]
	} else {
		issue.Path = "config"
		issue.Message = line
	}

	// Determine type based on path or message content
	pathLower := strings.ToLower(issue.Path)
	msgLower := strings.ToLower(issue.Message)

	switch {
	case strings.Contains(msgLower, "cel"):
		issue.Type = "cel"
	case strings.Contains(pathLower, "expression"):
		issue.Type = "cel"
	case strings.Contains(msgLower, "template variable"):
		issue.Type = "template"
	case strings.Contains(pathLower, "manifest"):
		issue.Type = "manifest"
	case strings.Contains(msgLower, "kubernetes"):
		issue.Type = "manifest"
	case strings.Contains(pathLower, "param"):
		issue.Type = "param"
	case strings.Contains(msgLower, "operator"):
		issue.Type = "param"
	default:
		issue.Type = "schema"
	}

	return issue
}

// categorizeIssue adds an issue to the appropriate category
func categorizeIssue(issue ValidationIssue, result *ValidationResult) {
	switch issue.Type {
	case "cel":
		result.Details.CEL.Passed = false
		result.Details.CEL.Issues = append(result.Details.CEL.Issues, issue)
	case "template":
		result.Details.Templates.Passed = false
		result.Details.Templates.Issues = append(result.Details.Templates.Issues, issue)
	case "manifest":
		result.Details.Manifests.Passed = false
		result.Details.Manifests.Issues = append(result.Details.Manifests.Issues, issue)
	case "param":
		result.Details.Params.Passed = false
		result.Details.Params.Issues = append(result.Details.Params.Issues, issue)
	default:
		result.Details.Schema.Passed = false
		result.Details.Schema.Issues = append(result.Details.Schema.Issues, issue)
	}
}

// countConfigItems counts various items in the config for reporting
func countConfigItems(config *config_loader.AdapterConfig, result *ValidationResult) {
	// Count parameters
	result.Details.Params.Count = len(config.Spec.Params)

	// Count CEL expressions
	celCount := 0
	for _, precond := range config.Spec.Preconditions {
		if precond.Expression != "" {
			celCount++
		}
	}
	result.Details.CEL.Count = celCount

	// Count resources (manifests)
	result.Details.Manifests.Count = len(config.Spec.Resources)

	// Count template usages (approximate - just count resources and payloads)
	templateCount := len(config.Spec.Resources)
	if config.Spec.Post != nil {
		templateCount += len(config.Spec.Post.Payloads)
	}
	result.Details.Templates.Count = templateCount
}

// ValidateConfigWithOptions performs validation with additional options
type ValidateOptions struct {
	// Strict treats warnings as errors
	Strict bool
	// Verbose includes additional checks
	Verbose bool
}

// ValidateConfigWithOpts validates config with options
func ValidateConfigWithOpts(configPath string, opts ValidateOptions) (*ValidationResult, error) {
	result, err := ValidateConfig(configPath)
	if err != nil {
		return nil, err
	}

	if opts.Strict && len(result.Warnings) > 0 {
		result.Valid = false
		for _, warn := range result.Warnings {
			result.Errors = append(result.Errors, ValidationIssue{
				Path:    warn.Path,
				Message: fmt.Sprintf("[strict] %s", warn.Message),
				Type:    warn.Type,
			})
		}
	}

	return result, nil
}
