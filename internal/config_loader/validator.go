package config_loader

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/cel-go/cel"
)

// -----------------------------------------------------------------------------
// Validation Errors
// -----------------------------------------------------------------------------

// ValidationError represents a validation error with context
type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationErrors holds multiple validation errors
type ValidationErrors struct {
	Errors []ValidationError
}

func (ve *ValidationErrors) Error() string {
	if len(ve.Errors) == 0 {
		return "no validation errors"
	}
	var msgs []string
	for _, e := range ve.Errors {
		msgs = append(msgs, e.Error())
	}
	return fmt.Sprintf("validation failed with %d error(s):\n  - %s", len(ve.Errors), strings.Join(msgs, "\n  - "))
}

func (ve *ValidationErrors) Add(path, message string) {
	ve.Errors = append(ve.Errors, ValidationError{Path: path, Message: message})
}

func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// -----------------------------------------------------------------------------
// Validator
// -----------------------------------------------------------------------------

// Validator performs semantic validation on AdapterConfig.
// It validates template variables, CEL expressions, and K8s manifests.
type Validator struct {
	config        *AdapterConfig
	errors        *ValidationErrors
	definedParams map[string]bool
	celEnv        *cel.Env
}

// newValidator creates a new Validator for the given config
func newValidator(config *AdapterConfig) *Validator {
	return &Validator{
		config: config,
		errors: &ValidationErrors{},
	}
}

// Validate performs all semantic validations and returns any errors.
// This is the main entry point for validation.
func (v *Validator) Validate() error {
	if v.config == nil {
		return fmt.Errorf("config is nil")
	}

	// Initialize validation context
	v.collectDefinedParameters()
	if err := v.initCELEnv(); err != nil {
		v.errors.Add("cel", fmt.Sprintf("failed to create CEL environment: %v", err))
	}

	// Run all validators
	v.validateCaptureFields()
	v.validateTemplateVariables()
	v.validateCELExpressions()
	v.validateK8sManifests()

	if v.errors.HasErrors() {
		return v.errors
	}
	return nil
}

// -----------------------------------------------------------------------------
// Parameter Collection
// -----------------------------------------------------------------------------

// collectDefinedParameters collects all defined parameter names for template validation
func (v *Validator) collectDefinedParameters() {
	v.definedParams = v.config.GetDefinedVariables()
}

// -----------------------------------------------------------------------------
// Capture Field Validation
// -----------------------------------------------------------------------------

// validateCaptureFields validates capture fields in API call steps
func (v *Validator) validateCaptureFields() {
	for i, step := range v.config.Spec.Steps {
		if step.APICall != nil {
			for j, capture := range step.APICall.Capture {
				path := fmt.Sprintf("%s.%s[%d].%s.%s[%d]", FieldSpec, FieldSteps, i, FieldAPICall, FieldCapture, j)
				v.validateCaptureField(capture, path)
			}
		}
	}
}

// validateCaptureField validates a single capture field configuration
func (v *Validator) validateCaptureField(capture CaptureField, path string) {
	// Name is required
	if capture.Name == "" {
		v.errors.Add(path, "capture name is required")
	}

	hasField := capture.Field != ""
	hasExpression := capture.Expression != ""

	// Must have exactly one of field or expression
	if !hasField && !hasExpression {
		v.errors.Add(path, "capture must have either 'field' or 'expression' set")
	} else if hasField && hasExpression {
		v.errors.Add(path, "capture cannot have both 'field' and 'expression' set; use only one")
	}

	// If expression is set, validate it as CEL
	if hasExpression && v.celEnv != nil {
		v.validateCELExpression(capture.Expression, path+"."+FieldExpression)
	}
}

// -----------------------------------------------------------------------------
// Template Variable Validation
// -----------------------------------------------------------------------------

// templateVarRegex matches Go template variables like {{ .varName }} or {{ .nested.var }}
var templateVarRegex = regexp.MustCompile(`\{\{\s*\.([a-zA-Z_][a-zA-Z0-9_\.]*)\s*(?:\|[^}]*)?\}\}`)

// validateTemplateVariables validates that template variables are defined
func (v *Validator) validateTemplateVariables() {
	// Validate step configurations
	for i, step := range v.config.Spec.Steps {
		basePath := fmt.Sprintf("%s.%s[%d]", FieldSpec, FieldSteps, i)

		// Validate API call step templates
		if step.APICall != nil {
			apiPath := basePath + "." + FieldAPICall
			v.validateTemplateString(step.APICall.URL, apiPath+"."+FieldURL)
			v.validateTemplateString(step.APICall.Body, apiPath+"."+FieldBody)
			for j, header := range step.APICall.Headers {
				v.validateTemplateString(header.Value,
					fmt.Sprintf("%s.%s[%d].%s", apiPath, FieldHeaders, j, FieldValue))
			}
		}

		// Validate resource step templates
		if step.Resource != nil {
			resourcePath := basePath + "." + FieldResource
			if manifest, ok := step.Resource.Manifest.(map[string]interface{}); ok {
				v.validateTemplateMap(manifest, resourcePath+"."+FieldManifest)
			}
			if step.Resource.Discovery != nil {
				discoveryPath := resourcePath + "." + FieldDiscovery
				v.validateTemplateString(step.Resource.Discovery.Namespace, discoveryPath+"."+FieldNamespace)
				v.validateTemplateString(step.Resource.Discovery.ByName, discoveryPath+"."+FieldByName)
				if step.Resource.Discovery.BySelectors != nil {
					for k, val := range step.Resource.Discovery.BySelectors.LabelSelector {
						v.validateTemplateString(val,
							fmt.Sprintf("%s.%s.%s[%s]", discoveryPath, FieldBySelectors, FieldLabelSelector, k))
					}
				}
			}
		}

		// Validate log step templates
		if step.Log != nil {
			v.validateTemplateString(step.Log.Message, basePath+"."+FieldLog+".message")
		}

		// Validate payload step templates (recursively)
		if step.Payload != nil {
			if payloadMap, ok := step.Payload.(map[string]interface{}); ok {
				v.validateTemplateMap(payloadMap, basePath+"."+FieldPayload)
			}
		}
	}
}

// validateTemplateString checks template variables in a string
func (v *Validator) validateTemplateString(s string, path string) {
	if s == "" {
		return
	}

	matches := templateVarRegex.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		if len(match) > 1 {
			varName := match[1]
			if !v.isVariableDefined(varName) {
				v.errors.Add(path, fmt.Sprintf("undefined template variable %q", varName))
			}
		}
	}
}

// isVariableDefined checks if a variable is defined (including nested paths)
func (v *Validator) isVariableDefined(varName string) bool {
	// Check exact match
	if v.definedParams[varName] {
		return true
	}

	// Check if the root variable is defined (for nested paths like clusterDetails.status.phase)
	parts := strings.Split(varName, ".")
	if len(parts) > 0 {
		root := parts[0]

		// Handle simple root variables (e.g. "metadata", "clusterId")
		if v.definedParams[root] {
			return true
		}
	}

	return false
}

// validateTemplateMap recursively validates template variables in a map
func (v *Validator) validateTemplateMap(m map[string]interface{}, path string) {
	for key, value := range m {
		currentPath := fmt.Sprintf("%s.%s", path, key)
		switch val := value.(type) {
		case string:
			v.validateTemplateString(val, currentPath)
		case map[string]interface{}:
			v.validateTemplateMap(val, currentPath)
		case []interface{}:
			for i, item := range val {
				itemPath := fmt.Sprintf("%s[%d]", currentPath, i)
				if str, ok := item.(string); ok {
					v.validateTemplateString(str, itemPath)
				} else if m, ok := item.(map[string]interface{}); ok {
					v.validateTemplateMap(m, itemPath)
				}
			}
		}
	}
}

// -----------------------------------------------------------------------------
// CEL Expression Validation
// -----------------------------------------------------------------------------

// initCELEnv initializes the CEL environment dynamically from config-defined variables.
// This uses v.definedParams which must be populated by collectDefinedParameters() first.
func (v *Validator) initCELEnv() error {
	// Pre-allocate capacity: +2 for cel.OptionalTypes() and potential "adapter" variable
	options := make([]cel.EnvOption, 0, len(v.definedParams)+2)

	// Enable optional types for optional chaining syntax (e.g., a.?b.?c)
	options = append(options, cel.OptionalTypes())

	// Track root variables we've already added (to avoid duplicates for nested paths)
	addedRoots := make(map[string]bool)

	for varName := range v.definedParams {
		// Extract root variable name (e.g., "clusterDetails" from "clusterDetails.status.phase")
		root := varName
		if idx := strings.Index(varName, "."); idx > 0 {
			root = varName[:idx]
		}

		// Skip if we've already added this root variable
		if addedRoots[root] {
			continue
		}
		addedRoots[root] = true

		// Use DynType since we don't know the actual type at validation time
		options = append(options, cel.Variable(root, cel.DynType))
	}

	// Always add "adapter" as a map for adapter metadata lookups like adapter.executionStatus
	if !addedRoots[FieldAdapter] {
		options = append(options, cel.Variable(FieldAdapter, cel.MapType(cel.StringType, cel.DynType)))
	}

	env, err := cel.NewEnv(options...)
	if err != nil {
		return err
	}
	v.celEnv = env
	return nil
}

// validateCELExpressions validates all CEL expressions in the config
func (v *Validator) validateCELExpressions() {
	if v.celEnv == nil {
		return // CEL env initialization failed, already reported
	}

	// Validate step 'when' clauses and param expressions
	for i, step := range v.config.Spec.Steps {
		basePath := fmt.Sprintf("%s.%s[%d]", FieldSpec, FieldSteps, i)

		// Validate 'when' clause if present
		if step.When != "" {
			v.validateCELExpression(step.When, basePath+"."+FieldWhen)
		}

		// Validate param expression
		if step.Param != nil && step.Param.Expression != "" {
			v.validateCELExpression(step.Param.Expression, basePath+"."+FieldParam+"."+FieldExpression)
		}

		// Validate payload expressions (recursively)
		if step.Payload != nil {
			if payloadMap, ok := step.Payload.(map[string]interface{}); ok {
				v.validatePayloadExpressions(payloadMap, basePath+"."+FieldPayload)
			}
		}
	}
}

// validateCELExpression validates a single CEL expression (syntax only)
// Type checking is skipped because variables are dynamic (DynType) and
// their actual types are only known at runtime.
func (v *Validator) validateCELExpression(expr string, path string) {
	if expr == "" {
		return
	}

	// Clean up the expression (remove leading/trailing whitespace and newlines)
	expr = strings.TrimSpace(expr)

	// Syntax validation only
	_, issues := v.celEnv.Parse(expr)
	if issues != nil && issues.Err() != nil {
		v.errors.Add(path, fmt.Sprintf("CEL parse error: %v", issues.Err()))
	}
}

// validatePayloadExpressions recursively validates CEL expressions in a payload structure.
// It looks for any field named "expression" and validates it as a CEL expression.
func (v *Validator) validatePayloadExpressions(m map[string]interface{}, path string) {
	for key, value := range m {
		currentPath := fmt.Sprintf("%s.%s", path, key)
		switch val := value.(type) {
		case string:
			// If the key is "expression", validate it as CEL
			if key == FieldExpression {
				v.validateCELExpression(val, currentPath)
			}
		case map[string]interface{}:
			v.validatePayloadExpressions(val, currentPath)
		case []interface{}:
			for i, item := range val {
				itemPath := fmt.Sprintf("%s[%d]", currentPath, i)
				if m, ok := item.(map[string]interface{}); ok {
					v.validatePayloadExpressions(m, itemPath)
				}
			}
		}
	}
}

// -----------------------------------------------------------------------------
// Kubernetes Manifest Validation
// -----------------------------------------------------------------------------

// validateK8sManifests validates Kubernetes resource manifests in resource steps
func (v *Validator) validateK8sManifests() {
	for i, step := range v.config.Spec.Steps {
		if step.Resource == nil {
			continue
		}

		path := fmt.Sprintf("%s.%s[%d].%s.%s", FieldSpec, FieldSteps, i, FieldResource, FieldManifest)

		// Validate inline manifest
		if manifest, ok := step.Resource.Manifest.(map[string]interface{}); ok {
			v.validateK8sManifest(manifest, path)
		}
	}
}

// validateK8sManifest validates a single Kubernetes manifest
func (v *Validator) validateK8sManifest(manifest map[string]interface{}, path string) {
	// Required fields for K8s resources
	requiredFields := []string{FieldAPIVersion, FieldKind, FieldMetadata}

	for _, field := range requiredFields {
		if _, ok := manifest[field]; !ok {
			v.errors.Add(path, fmt.Sprintf("missing required Kubernetes field %q", field))
		}
	}

	// Validate metadata has name
	if metadata, ok := manifest[FieldMetadata].(map[string]interface{}); ok {
		if _, hasName := metadata[FieldName]; !hasName {
			v.errors.Add(path+"."+FieldMetadata, fmt.Sprintf("missing required field %q", FieldName))
		}
	}

	// Validate apiVersion format
	if apiVersion, ok := manifest[FieldAPIVersion].(string); ok {
		if apiVersion == "" {
			v.errors.Add(path+"."+FieldAPIVersion, "apiVersion cannot be empty")
		}
	}

	// Validate kind
	if kind, ok := manifest[FieldKind].(string); ok {
		if kind == "" {
			v.errors.Add(path+"."+FieldKind, "kind cannot be empty")
		}
	}
}
