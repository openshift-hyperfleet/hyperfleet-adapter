package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
)

// ParamExtractionPhase handles parameter extraction from events and environment
type ParamExtractionPhase struct {
	config    *config_loader.Config
	k8sClient k8s_client.K8sClient
	log       logger.Logger
}

// NewParamExtractionPhase creates a new parameter extraction phase
func NewParamExtractionPhase(config *config_loader.Config, k8sClient k8s_client.K8sClient, log logger.Logger) *ParamExtractionPhase {
	return &ParamExtractionPhase{
		config:    config,
		k8sClient: k8sClient,
		log:       log,
	}
}

// Name returns the phase identifier
func (p *ParamExtractionPhase) Name() ExecutionPhase {
	return PhaseParamExtraction
}

// ShouldSkip determines if this phase should be skipped
func (p *ParamExtractionPhase) ShouldSkip(execCtx *ExecutionContext) (bool, string) {
	// Parameter extraction is always executed
	return false, ""
}

// Execute runs parameter extraction logic
func (p *ParamExtractionPhase) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	// Extract configured parameters
	if err := p.extractConfigParams(execCtx); err != nil {
		execCtx.SetError("ParameterExtractionFailed", err.Error())
		return err
	}

	// Add metadata params
	p.addMetadataParams(execCtx)

	p.log.Debugf(ctx, "Parameter extraction completed: extracted %d params", len(execCtx.Params))
	return nil
}

// extractConfigParams extracts all configured parameters and populates execCtx.Params
func (p *ParamExtractionPhase) extractConfigParams(execCtx *ExecutionContext) error {
	for _, param := range p.config.Spec.Params {
		value, err := p.extractParam(execCtx.Ctx, param, execCtx.EventData)
		if err != nil {
			if param.Required {
				return NewExecutorError(PhaseParamExtraction, param.Name,
					fmt.Sprintf("failed to extract required parameter '%s' from source '%s'", param.Name, param.Source), err)
			}
			// Use default for non-required params if extraction fails
			if param.Default != nil {
				execCtx.Params[param.Name] = param.Default
			}
			continue
		}

		// Apply default if value is nil or (for strings) empty
		isEmpty := value == nil
		if s, ok := value.(string); ok && s == "" {
			isEmpty = true
		}
		if isEmpty && param.Default != nil {
			value = param.Default
		}

		// Apply type conversion if specified
		if value != nil && param.Type != "" {
			converted, convErr := convertParamType(value, param.Type)
			if convErr != nil {
				if param.Required {
					return NewExecutorError(PhaseParamExtraction, param.Name,
						fmt.Sprintf("failed to convert parameter '%s' to type '%s'", param.Name, param.Type), convErr)
				}
				// Use default for non-required params if conversion fails
				if param.Default != nil {
					execCtx.Params[param.Name] = param.Default
				}
				continue
			}
			value = converted
		}

		if value != nil {
			execCtx.Params[param.Name] = value
		}
	}

	return nil
}

// extractParam extracts a single parameter based on its source
func (p *ParamExtractionPhase) extractParam(ctx context.Context, param config_loader.Parameter, eventData map[string]interface{}) (interface{}, error) {
	source := param.Source

	// Handle different source types
	switch {
	case strings.HasPrefix(source, "env."):
		return extractFromEnv(source[4:])
	case strings.HasPrefix(source, "event."):
		return extractFromEvent(source[6:], eventData)
	case strings.HasPrefix(source, "secret."):
		return p.extractFromSecret(ctx, source[7:])
	case strings.HasPrefix(source, "configmap."):
		return p.extractFromConfigMap(ctx, source[10:])
	case source == "":
		// No source specified, return default or nil
		return param.Default, nil
	default:
		// Try to extract from event data directly
		return extractFromEvent(source, eventData)
	}
}

// extractFromEnv extracts a value from environment variables.
// This delegates to utils.GetEnvOrError.
func extractFromEnv(envVar string) (interface{}, error) {
	return utils.GetEnvOrError(envVar)
}

// extractFromEvent extracts a value from event data using dot notation.
// This delegates to utils.GetNestedValue.
func extractFromEvent(path string, eventData map[string]interface{}) (interface{}, error) {
	return utils.GetNestedValue(eventData, path)
}

// extractFromSecret extracts a value from a Kubernetes Secret
// Format: secret.<namespace>.<secret-name>.<key> (namespace is required)
func (p *ParamExtractionPhase) extractFromSecret(ctx context.Context, path string) (interface{}, error) {
	if p.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured, cannot extract from secret")
	}

	value, err := p.k8sClient.ExtractFromSecret(ctx, path)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// extractFromConfigMap extracts a value from a Kubernetes ConfigMap
// Format: configmap.<namespace>.<configmap-name>.<key> (namespace is required)
func (p *ParamExtractionPhase) extractFromConfigMap(ctx context.Context, path string) (interface{}, error) {
	if p.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured, cannot extract from configmap")
	}

	value, err := p.k8sClient.ExtractFromConfigMap(ctx, path)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// addMetadataParams adds adapter and event metadata to execCtx.Params
func (p *ParamExtractionPhase) addMetadataParams(execCtx *ExecutionContext) {
	// Add metadata from adapter config
	execCtx.Params["metadata"] = map[string]interface{}{
		"name":   p.config.Metadata.Name,
		"labels": p.config.Metadata.Labels,
	}
}

// convertParamType converts a value to the specified type.
// This delegates to utils.ConvertToType.
// Supported types: string, int, int64, float, float64, bool
func convertParamType(value interface{}, targetType string) (interface{}, error) {
	return utils.ConvertToType(value, targetType)
}
