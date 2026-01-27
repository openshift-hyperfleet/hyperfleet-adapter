package config_loader

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// validResourceNameRegex validates resource names for CEL compatibility.
// Allows snake_case (my_resource) and camelCase (myResource).
// Must start with lowercase letter, can contain letters, numbers, underscores.
// Hyphens (kebab-case) are NOT allowed as they conflict with CEL's minus operator.
var validResourceNameRegex = regexp.MustCompile(`^[a-z][a-zA-Z0-9_]*$`)

// -----------------------------------------------------------------------------
// SchemaValidator
// -----------------------------------------------------------------------------

// SchemaValidator performs schema validation on AdapterConfig.
// It validates required fields for the step-based execution model.
type SchemaValidator struct {
	config  *AdapterConfig
	baseDir string // Base directory for resolving relative paths
}

// NewSchemaValidator creates a new SchemaValidator for the given config
func NewSchemaValidator(config *AdapterConfig, baseDir string) *SchemaValidator {
	return &SchemaValidator{
		config:  config,
		baseDir: baseDir,
	}
}

// ValidateStructure performs all structural validations.
// Returns error on first validation failure (fail-fast).
func (v *SchemaValidator) ValidateStructure() error {
	validators := []func() error{
		v.validateAPIVersionAndKind,
		v.validateMetadata,
		v.validateAdapterSpec,
		v.validateSteps,
	}

	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}

	return nil
}

// ValidateFileReferences validates that all file references exist.
// Only runs if baseDir is set.
func (v *SchemaValidator) ValidateFileReferences() error {
	// Step-based model uses inline manifests, no file references to validate
	return nil
}

// LoadFileReferences loads content from file references into the config.
// Only runs if baseDir is set.
func (v *SchemaValidator) LoadFileReferences() error {
	// Step-based model uses inline manifests, no file references to load
	return nil
}

// -----------------------------------------------------------------------------
// Core Structural Validators
// -----------------------------------------------------------------------------

func (v *SchemaValidator) validateAPIVersionAndKind() error {
	if v.config.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if !IsSupportedAPIVersion(v.config.APIVersion) {
		return fmt.Errorf("unsupported apiVersion %q (supported: %s)",
			v.config.APIVersion, strings.Join(SupportedAPIVersions, ", "))
	}
	if v.config.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if v.config.Kind != ExpectedKind {
		return fmt.Errorf("invalid kind %q (expected: %q)", v.config.Kind, ExpectedKind)
	}
	return nil
}

func (v *SchemaValidator) validateMetadata() error {
	if v.config.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	return nil
}

func (v *SchemaValidator) validateAdapterSpec() error {
	if v.config.Spec.Adapter.Version == "" {
		return fmt.Errorf("%s.%s.%s is required", FieldSpec, FieldAdapter, FieldVersion)
	}
	return nil
}

// validateSteps validates the step-based execution model
func (v *SchemaValidator) validateSteps() error {
	if len(v.config.Spec.Steps) == 0 {
		return nil // Empty steps is valid (no-op adapter)
	}

	seen := make(map[string]bool)

	for i, step := range v.config.Spec.Steps {
		path := fmt.Sprintf("%s.steps[%d]", FieldSpec, i)

		// Name is required
		if step.Name == "" {
			return fmt.Errorf("%s.name is required", path)
		}

		// Validate name format (same rules as resource names for CEL)
		if !validResourceNameRegex.MatchString(step.Name) {
			return fmt.Errorf("%s.name: %q must start with lowercase letter and contain only letters, numbers, underscores (no hyphens)", path, step.Name)
		}

		// Check for duplicate names
		if seen[step.Name] {
			return fmt.Errorf("%s: duplicate step name %q", path, step.Name)
		}
		seen[step.Name] = true

		// Exactly one step type must be set
		typeCount := step.CountStepTypes()
		if typeCount == 0 {
			return fmt.Errorf("%s (%s): must specify one of: param, apiCall, resource, payload, log", path, step.Name)
		}
		if typeCount > 1 {
			return fmt.Errorf("%s (%s): can only specify one step type, found %d", path, step.Name, typeCount)
		}

		// Validate step-type-specific fields
		if err := v.validateStepFields(step, path); err != nil {
			return err
		}
	}

	return nil
}

// validateStepFields validates the fields specific to each step type
func (v *SchemaValidator) validateStepFields(step Step, path string) error {
	switch {
	case step.Param != nil:
		return v.validateParamStep(step.Param, path+".param", step.Name)
	case step.APICall != nil:
		return v.validateAPICallStep(step.APICall, path+".apiCall", step.Name)
	case step.Resource != nil:
		return v.validateResourceStep(step.Resource, path+".resource", step.Name)
	case step.Log != nil:
		return v.validateLogStep(step.Log, path+".log", step.Name)
	}
	// payload doesn't need special validation - it's flexible
	return nil
}

func (v *SchemaValidator) validateParamStep(ps *ParamStep, path, stepName string) error {
	// At least one of source, value, or expression should be set (or none if using default)
	// This is optional validation - we allow empty param steps with just a default
	return nil
}

func (v *SchemaValidator) validateAPICallStep(as *APICallStep, path, stepName string) error {
	if as.Method == "" {
		return fmt.Errorf("%s (%s): method is required", path, stepName)
	}
	if as.URL == "" {
		return fmt.Errorf("%s (%s): url is required", path, stepName)
	}
	// Validate HTTP method
	validMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	if !slices.Contains(validMethods, strings.ToUpper(as.Method)) {
		return fmt.Errorf("%s (%s): invalid method %q (must be one of: %s)", path, stepName, as.Method, strings.Join(validMethods, ", "))
	}
	return nil
}

func (v *SchemaValidator) validateResourceStep(rs *ResourceStep, path, stepName string) error {
	if rs.Manifest == nil {
		return fmt.Errorf("%s (%s): manifest is required", path, stepName)
	}
	if rs.Discovery == nil {
		return fmt.Errorf("%s (%s): discovery is required", path, stepName)
	}
	// Validate discovery has either byName or bySelectors
	if rs.Discovery.ByName == "" && rs.Discovery.BySelectors == nil {
		return fmt.Errorf("%s.discovery (%s): must have either byName or bySelectors", path, stepName)
	}
	// Validate bySelectors has labelSelector if specified
	if rs.Discovery.BySelectors != nil && len(rs.Discovery.BySelectors.LabelSelector) == 0 {
		return fmt.Errorf("%s.discovery.bySelectors (%s): must have labelSelector defined", path, stepName)
	}
	return nil
}

func (v *SchemaValidator) validateLogStep(ls *LogStep, path, stepName string) error {
	if ls.Message == "" {
		return fmt.Errorf("%s (%s): message is required", path, stepName)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Package-level Helper Functions
// -----------------------------------------------------------------------------

// IsSupportedAPIVersion checks if the given apiVersion is supported
func IsSupportedAPIVersion(apiVersion string) bool {
	for _, v := range SupportedAPIVersions {
		if v == apiVersion {
			return true
		}
	}
	return false
}

// ValidateAdapterVersion validates the config's adapter version matches the expected version
func ValidateAdapterVersion(config *AdapterConfig, expectedVersion string) error {
	if expectedVersion == "" {
		return nil
	}

	configVersion := config.Spec.Adapter.Version
	if configVersion != expectedVersion {
		return fmt.Errorf("adapter version mismatch: config %q != adapter %q",
			configVersion, expectedVersion)
	}

	return nil
}

// -----------------------------------------------------------------------------
// Legacy Functions (for backward compatibility with loader.go)
// -----------------------------------------------------------------------------

func validateAPIVersionAndKind(config *AdapterConfig) error {
	return NewSchemaValidator(config, "").validateAPIVersionAndKind()
}

func validateMetadata(config *AdapterConfig) error {
	return NewSchemaValidator(config, "").validateMetadata()
}

func validateAdapterSpec(config *AdapterConfig) error {
	return NewSchemaValidator(config, "").validateAdapterSpec()
}

func validateSteps(config *AdapterConfig) error {
	return NewSchemaValidator(config, "").validateSteps()
}

func validateFileReferences(config *AdapterConfig, baseDir string) error {
	return NewSchemaValidator(config, baseDir).ValidateFileReferences()
}

func loadFileReferences(config *AdapterConfig, baseDir string) error {
	return NewSchemaValidator(config, baseDir).LoadFileReferences()
}
