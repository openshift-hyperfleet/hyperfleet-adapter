package executor

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/resource_applier"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourcesPhase handles Kubernetes resource creation/update.
// It uses the ResourceApplier interface to support multiple backends:
//   - Direct Kubernetes API (k8s_client)
//   - Maestro/OCM ManifestWork (maestro_client)
type ResourcesPhase struct {
	applier resource_applier.ResourceApplier
	config  *config_loader.Config
	log     logger.Logger
	// results stores the resource operation results for later retrieval
	results []ResourceResult
}

// NewResourcesPhase creates a new resources phase.
// The applier parameter can be any implementation of ResourceApplier:
//   - k8s_client.Client for direct Kubernetes API access
//   - maestro_client.ResourceApplier for ManifestWork-based deployments
func NewResourcesPhase(applier resource_applier.ResourceApplier, config *config_loader.Config, log logger.Logger) *ResourcesPhase {
	return &ResourcesPhase{
		applier: applier,
		config:  config,
		log:     log,
	}
}

// Name returns the phase identifier
func (p *ResourcesPhase) Name() ExecutionPhase {
	return PhaseResources
}

// ShouldSkip determines if this phase should be skipped
func (p *ResourcesPhase) ShouldSkip(execCtx *ExecutionContext) (bool, string) {
	// Skip if no resources configured
	if len(p.config.Spec.Resources) == 0 {
		return true, "no resources configured"
	}

	// Skip if resources were marked to be skipped (e.g., precondition not met)
	if execCtx.Adapter.ResourcesSkipped {
		return true, execCtx.Adapter.SkipReason
	}

	return false, ""
}

// Execute runs resource creation/update logic
func (p *ResourcesPhase) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	p.log.Infof(ctx, "Processing %d resources", len(p.config.Spec.Resources))

	results, err := p.executeAll(ctx, p.config.Spec.Resources, execCtx)
	p.results = results

	if err != nil {
		execCtx.SetError("ResourceFailed", err.Error())
		return fmt.Errorf("resource execution failed: %w", err)
	}

	p.log.Infof(ctx, "Successfully processed %d resources", len(results))
	return nil
}

// Results returns the resource operation results
func (p *ResourcesPhase) Results() []ResourceResult {
	return p.results
}

// executeAll creates/updates all resources using the ResourceApplier.
// It prepares all resources first (build manifests, discover existing), then applies them in batch.
// Returns results for each resource and updates the execution context.
func (p *ResourcesPhase) executeAll(ctx context.Context, resources []config_loader.Resource, execCtx *ExecutionContext) ([]ResourceResult, error) {
	if execCtx.Resources == nil {
		execCtx.Resources = make(map[string]*unstructured.Unstructured)
	}

	if p.applier == nil {
		return nil, fmt.Errorf("resource applier not configured")
	}

	// Phase 1: Prepare all resources (build manifests, discover existing)
	p.log.Infof(ctx, "Preparing %d resources...", len(resources))
	resourcesToApply, prepResults, err := p.prepareResources(ctx, resources, execCtx)
	if err != nil {
		return prepResults, err
	}

	// Phase 2: Apply all resources using the ResourceApplier
	p.log.Infof(ctx, "Applying %d resources...", len(resourcesToApply))
	applyResults, err := p.applier.ApplyResources(ctx, resourcesToApply)
	if err != nil {
		// Convert apply results to ResourceResult and set error context
		results := p.convertApplyResults(ctx, applyResults, prepResults, execCtx)
		return results, err
	}

	// Convert successful results
	results := p.convertApplyResults(ctx, applyResults, prepResults, execCtx)
	return results, nil
}

// prepareResources builds manifests and discovers existing resources for all resources.
// Returns the prepared ResourceToApply list and partial results if any preparation fails.
func (p *ResourcesPhase) prepareResources(ctx context.Context, resources []config_loader.Resource, execCtx *ExecutionContext) ([]resource_applier.ResourceToApply, []ResourceResult, error) {
	resourcesToApply := make([]resource_applier.ResourceToApply, 0, len(resources))
	results := make([]ResourceResult, 0, len(resources))

	for _, resource := range resources {
		result := ResourceResult{
			Name:   resource.Name,
			Status: StatusSuccess,
		}

		// Step 1: Build the manifest
		p.log.Debugf(ctx, "Building manifest for resource[%s]...", resource.Name)
		obj, err := p.buildManifest(ctx, resource, execCtx)
		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			results = append(results, result)
			return resourcesToApply, results, NewExecutorError(PhaseResources, resource.Name, "failed to build manifest", err)
		}

		// Extract resource info
		gvk := obj.GroupVersionKind()
		result.Kind = gvk.Kind
		result.Namespace = obj.GetNamespace()
		result.ResourceName = obj.GetName()

		p.log.Debugf(ctx, "Resource[%s] manifest built: %s/%s in namespace %s",
			resource.Name, gvk.Kind, obj.GetName(), obj.GetNamespace())

		// Step 2: Discover existing resource
		existingResource, err := p.discoverExisting(ctx, resource, obj, execCtx)
		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			results = append(results, result)
			return resourcesToApply, results, NewExecutorError(PhaseResources, resource.Name, "failed to discover existing resource", err)
		}

		if existingResource != nil {
			p.log.Debugf(ctx, "Resource[%s] existing found: %s/%s",
				resource.Name, existingResource.GetNamespace(), existingResource.GetName())
		} else {
			p.log.Debugf(ctx, "Resource[%s] no existing found, will create", resource.Name)
		}

		// Add to batch
		resourcesToApply = append(resourcesToApply, resource_applier.ResourceToApply{
			Name:     resource.Name,
			Manifest: obj,
			Existing: existingResource,
			Options: &resource_applier.ApplyOptions{
				RecreateOnChange: resource.RecreateOnChange,
			},
		})
		results = append(results, result)
	}

	return resourcesToApply, results, nil
}

// discoverExisting discovers the existing resource using Discovery config or by name.
func (p *ResourcesPhase) discoverExisting(ctx context.Context, resource config_loader.Resource, obj *unstructured.Unstructured, execCtx *ExecutionContext) (*unstructured.Unstructured, error) {
	gvk := obj.GroupVersionKind()

	var existingResource *unstructured.Unstructured
	var err error

	if resource.Discovery != nil {
		// Use Discovery config to find existing resource (e.g., by label selector)
		existingResource, err = p.discoverExistingResource(ctx, gvk, resource.Discovery, execCtx)
	} else {
		// No Discovery config - lookup by name from manifest
		existingResource, err = p.applier.GetResource(ctx, gvk, obj.GetNamespace(), obj.GetName())
	}

	// NotFound is not an error - it means the resource doesn't exist yet
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	return existingResource, nil
}

// convertApplyResults converts resource_applier.ApplyResourcesResult to []ResourceResult.
// It also updates the execution context with the resulting resources.
func (p *ResourcesPhase) convertApplyResults(ctx context.Context, applyResults *resource_applier.ApplyResourcesResult, prepResults []ResourceResult, execCtx *ExecutionContext) []ResourceResult {
	results := make([]ResourceResult, len(prepResults))
	copy(results, prepResults)

	for i, applyResult := range applyResults.Results {
		if i >= len(results) {
			break
		}

		// Update result with apply outcome
		if applyResult.Error != nil {
			results[i].Status = StatusFailed
			results[i].Error = applyResult.Error
			// Set ExecutionError for the first failure
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhaseResources),
				Step:    applyResult.Name,
				Message: applyResult.Error.Error(),
			}
			errCtx := logger.WithK8sResult(ctx, "FAILED")
			errCtx = logger.WithErrorField(errCtx, applyResult.Error)
			p.log.Errorf(errCtx, "Resource[%s] processed: FAILED", applyResult.Name)
		} else if applyResult.ApplyResult != nil {
			results[i].Operation = applyResult.Operation
			results[i].OperationReason = applyResult.Reason
			results[i].Resource = applyResult.Resource

			// Store resource in execution context
			if results[i].Resource != nil {
				execCtx.Resources[applyResult.Name] = results[i].Resource
			}

			successCtx := logger.WithK8sResult(ctx, "SUCCESS")
			p.log.Infof(successCtx, "Resource[%s] processed: operation=%s reason=%s",
				applyResult.Name, results[i].Operation, results[i].OperationReason)
		}
	}

	return results
}

// buildManifest builds an unstructured manifest from the resource configuration
func (p *ResourcesPhase) buildManifest(ctx context.Context, resource config_loader.Resource, execCtx *ExecutionContext) (*unstructured.Unstructured, error) {
	var manifestData map[string]interface{}

	// Get manifest (inline or loaded from ref)
	if resource.Manifest != nil {
		switch m := resource.Manifest.(type) {
		case map[string]interface{}:
			manifestData = m
		case map[interface{}]interface{}:
			manifestData = utils.ConvertToStringKeyMap(m)
		default:
			return nil, fmt.Errorf("unsupported manifest type: %T", resource.Manifest)
		}
	} else {
		return nil, fmt.Errorf("no manifest specified for resource %s", resource.Name)
	}

	// Deep copy to avoid modifying the original
	copiedData, err := utils.DeepCopyMap(manifestData)
	if err != nil {
		p.log.Warnf(ctx, "Failed to deep copy manifest data: %v. Using shallow copy.", err)
		copiedData = utils.DeepCopyMapWithFallback(manifestData)
	}

	// Render all template strings in the manifest
	renderedData, err := manifest.RenderManifestData(copiedData, execCtx.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to render manifest templates: %w", err)
	}

	// Convert to unstructured
	obj := &unstructured.Unstructured{Object: renderedData}

	// Validate manifest (K8s fields + generation annotation)
	if err := manifest.ValidateManifest(obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// discoverExistingResource discovers an existing resource using the discovery config
func (p *ResourcesPhase) discoverExistingResource(ctx context.Context, gvk schema.GroupVersionKind, discovery *config_loader.DiscoveryConfig, execCtx *ExecutionContext) (*unstructured.Unstructured, error) {
	if p.applier == nil {
		return nil, fmt.Errorf("resource applier not configured")
	}

	// Render discovery namespace template
	// Empty namespace means all namespaces (normalized from "*" at config load time)
	namespace, err := utils.RenderTemplate(discovery.Namespace, execCtx.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to render namespace template: %w", err)
	}

	// Check if discovering by name
	if discovery.ByName != "" {
		name, err := utils.RenderTemplate(discovery.ByName, execCtx.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to render byName template: %w", err)
		}
		return p.applier.GetResource(ctx, gvk, namespace, name)
	}

	// Discover by label selector
	if discovery.BySelectors != nil && len(discovery.BySelectors.LabelSelector) > 0 {
		// Render label selector templates
		renderedLabels := make(map[string]string)
		for k, v := range discovery.BySelectors.LabelSelector {
			renderedK, err := utils.RenderTemplate(k, execCtx.Params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label key template: %w", err)
			}
			renderedV, err := utils.RenderTemplate(v, execCtx.Params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label value template: %w", err)
			}
			renderedLabels[renderedK] = renderedV
		}

		labelSelector := manifest.BuildLabelSelector(renderedLabels)

		discoveryConfig := &manifest.DiscoveryConfig{
			Namespace:     namespace,
			LabelSelector: labelSelector,
		}

		list, err := p.applier.DiscoverResources(ctx, gvk, discoveryConfig)
		if err != nil {
			return nil, err
		}

		if len(list.Items) == 0 {
			return nil, apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, "")
		}

		return manifest.GetLatestGenerationFromList(list), nil
	}

	return nil, fmt.Errorf("discovery config must specify byName or bySelectors")
}

// GetResourceAsMap converts an unstructured resource to a map for CEL evaluation
func GetResourceAsMap(resource *unstructured.Unstructured) map[string]interface{} {
	if resource == nil {
		return nil
	}
	return resource.Object
}

// BuildResourcesMap builds a map of all resources for CEL evaluation.
// Resource names are used directly as keys (snake_case and camelCase both work in CEL).
// Name validation (no hyphens, no duplicates) is done at config load time.
func BuildResourcesMap(resources map[string]*unstructured.Unstructured) map[string]interface{} {
	result := make(map[string]interface{})
	for name, resource := range resources {
		if resource != nil {
			result[name] = resource.Object
		}
	}
	return result
}
