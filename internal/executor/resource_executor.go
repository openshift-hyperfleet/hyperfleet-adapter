package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/maestroclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transportclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceExecutor creates and updates Kubernetes resources
type ResourceExecutor struct {
	client transportclient.TransportClient
	log    logger.Logger
}

// newResourceExecutor creates a new resource executor
// NOTE: Caller (NewExecutor) is responsible for config validation
func newResourceExecutor(config *ExecutorConfig) *ResourceExecutor {
	return &ResourceExecutor{
		client: config.TransportClient,
		log:    config.Logger,
	}
}

// ExecuteAll creates/updates all resources in sequence
// Returns results for each resource and updates the execution context
func (re *ResourceExecutor) ExecuteAll(
	ctx context.Context,
	resources []configloader.Resource,
	execCtx *ExecutionContext,
) ([]ResourceResult, error) {
	if execCtx.Resources == nil {
		execCtx.Resources = make(map[string]interface{})
	}
	results := make([]ResourceResult, 0, len(resources))

	for _, resource := range resources {
		result, err := re.executeResource(ctx, resource, execCtx)
		results = append(results, result)

		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// executeResource creates or updates a single resource via the transport client.
// For k8s transport: renders manifest template → marshals to JSON → calls ApplyResource(bytes)
// For maestro transport: renders manifestWork template → marshals to JSON → calls ApplyResource(bytes)
func (re *ResourceExecutor) executeResource(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
) (ResourceResult, error) {
	result := ResourceResult{
		Name:   resource.Name,
		Status: StatusSuccess,
	}

	transportClient := re.client
	if transportClient == nil {
		result.Status = StatusFailed
		result.Error = fmt.Errorf("transport client not configured for %s", resource.GetTransportClient())
		return result, NewExecutorError(PhaseResources, resource.Name, "transport client not configured", result.Error)
	}

	// Step 1: Render the manifest/manifestWork to bytes
	re.log.Debugf(ctx, "Rendering manifest template for resource %s", resource.Name)
	renderedBytes, err := re.renderToBytes(resource, execCtx)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		return result, NewExecutorError(PhaseResources, resource.Name, "failed to render manifest", err)
	}

	// Step 2: Extract resource identity from rendered manifest for result reporting
	var obj unstructured.Unstructured
	if unmarshalErr := json.Unmarshal(renderedBytes, &obj.Object); unmarshalErr == nil {
		result.Kind = obj.GetKind()
		result.Namespace = obj.GetNamespace()
		result.ResourceName = obj.GetName()
	}

	// Step 3: Prepare apply options
	var applyOpts *transportclient.ApplyOptions
	if resource.RecreateOnChange {
		applyOpts = &transportclient.ApplyOptions{RecreateOnChange: true}
	}

	// Step 4: Build transport context (nil for k8s, *maestroclient.TransportContext for maestro)
	var transportTarget transportclient.TransportContext
	if resource.IsMaestroTransport() && resource.Transport.Maestro != nil {
		targetCluster, tplErr := utils.RenderTemplate(resource.Transport.Maestro.TargetCluster, execCtx.Params)
		if tplErr != nil {
			result.Status = StatusFailed
			result.Error = tplErr
			return result, NewExecutorError(PhaseResources, resource.Name, "failed to render targetCluster template", tplErr)
		}
		transportTarget = &maestroclient.TransportContext{
			ConsumerName: targetCluster,
		}
	}

	// Step 5: Call transport client ApplyResource with rendered bytes
	applyResult, err := transportClient.ApplyResource(ctx, renderedBytes, applyOpts, transportTarget)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err
		execCtx.Adapter.ExecutionError = &ExecutionError{
			Phase:   string(PhaseResources),
			Step:    resource.Name,
			Message: err.Error(),
		}
		errCtx := logger.WithK8sResult(ctx, "FAILED")
		errCtx = logger.WithErrorField(errCtx, err)
		re.log.Errorf(errCtx, "Resource[%s] processed: FAILED", resource.Name)
		return result, NewExecutorError(PhaseResources, resource.Name, "failed to apply resource", err)
	}

	// Step 6: Extract result
	result.Operation = applyResult.Operation
	result.OperationReason = applyResult.Reason

	successCtx := logger.WithK8sResult(ctx, "SUCCESS")
	re.log.Infof(successCtx, "Resource[%s] processed: operation=%s reason=%s",
		resource.Name, result.Operation, result.OperationReason)

	// Step 7: Post-apply discovery — find the applied resource and store in execCtx for CEL evaluation
	if resource.Discovery != nil {
		discovered, discoverErr := re.discoverResource(ctx, resource, execCtx, transportTarget)
		if discoverErr != nil {
			result.Status = StatusFailed
			result.Error = discoverErr
			execCtx.Adapter.ExecutionError = &ExecutionError{
				Phase:   string(PhaseResources),
				Step:    resource.Name,
				Message: discoverErr.Error(),
			}
			errCtx := logger.WithK8sResult(ctx, "FAILED")
			errCtx = logger.WithErrorField(errCtx, discoverErr)
			re.log.Errorf(errCtx, "Resource[%s] discovery after apply failed: %v", resource.Name, discoverErr)
			return result, NewExecutorError(
				PhaseResources, resource.Name, "failed to discover resource after apply", discoverErr)
		}
		if discovered != nil {
			// Always store the discovered top-level resource by resource name.
			// Nested discoveries are added as independent entries keyed by nested name.
			execCtx.Resources[resource.Name] = discovered
			re.log.Debugf(ctx, "Resource[%s] discovered and stored in context", resource.Name)

			// Step 8: Nested discoveries — find sub-resources within the discovered parent (e.g., ManifestWork)
			if len(resource.NestedDiscoveries) > 0 {
				nestedResults := re.discoverNestedResources(ctx, resource, execCtx, discovered)
				for nestedName, nestedObj := range nestedResults {
					if nestedName == resource.Name {
						re.log.Warnf(ctx,
							"Nested discovery %q has the same name as parent resource; skipping to avoid overwriting parent",
							nestedName)
						continue
					}
					if nestedObj == nil {
						continue
					}
					if _, exists := execCtx.Resources[nestedName]; exists {
						collisionErr := fmt.Errorf(
							"nested discovery key collision: %q already exists in context",
							nestedName,
						)
						result.Status = StatusFailed
						result.Error = collisionErr
						execCtx.Adapter.ExecutionError = &ExecutionError{
							Phase:   string(PhaseResources),
							Step:    resource.Name,
							Message: collisionErr.Error(),
						}
						return result, NewExecutorError(
							PhaseResources, resource.Name,
							"duplicate resource context key",
							collisionErr,
						)
					}
					execCtx.Resources[nestedName] = nestedObj
				}
				re.log.Debugf(ctx, "Resource[%s] discovered with %d nested resources added to context",
					resource.Name, len(nestedResults))
			}
		}
	}

	return result, nil
}

// renderToBytes renders the resource's manifest template to JSON bytes.
// The manifest holds either a K8s resource or a ManifestWork depending on transport type.
// Supports two manifest formats:
//   - map (inline YAML): template strings within values are rendered recursively
//   - string (raw template from ref file or YAML multiline): the entire string is rendered
//     as a Go template first, then parsed as YAML
func (re *ResourceExecutor) renderToBytes(
	resource configloader.Resource,
	execCtx *ExecutionContext,
) ([]byte, error) {
	if resource.Manifest == nil {
		return nil, fmt.Errorf("no manifest specified for resource %s", resource.Name)
	}

	manifestSource := resource.Manifest

	// String manifests: render Go templates first, then parse YAML.
	// This supports structural templates ({{ if }}, {{ range }}, etc.) that produce
	// YAML keys, list items, or entire blocks conditionally.
	if manifestStr, ok := manifestSource.(string); ok {
		return manifest.RenderStringManifest(manifestStr, execCtx.Params)
	}

	// Map manifests: render template strings within values recursively
	var manifestData map[string]interface{}
	switch m := manifestSource.(type) {
	case map[string]interface{}:
		manifestData = m
	case map[interface{}]interface{}:
		manifestData = utils.ConvertToStringKeyMap(m)
	default:
		return nil, fmt.Errorf("unsupported manifest type: %T", manifestSource)
	}

	// Render all template strings in the manifest
	// Note: RenderManifestData creates new maps/slices at every level,
	// so the original manifestData is never mutated.
	renderedData, err := manifest.RenderManifestData(manifestData, execCtx.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to render manifest templates: %w", err)
	}

	// Marshal to JSON bytes
	data, err := json.Marshal(renderedData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rendered manifest: %w", err)
	}

	return data, nil
}

// discoverResource discovers the applied resource using the discovery config.
// For k8s transport: discovers the K8s resource by name or label selector.
// For maestro transport: discovers the ManifestWork by name or label selector.
// The discovered resource is stored in execCtx.Resources for post-action CEL evaluation.
func (re *ResourceExecutor) discoverResource(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
	transportTarget transportclient.TransportContext,
) (*unstructured.Unstructured, error) {
	discovery := resource.Discovery
	if discovery == nil {
		return nil, nil
	}

	// Render discovery namespace template
	namespace, err := utils.RenderTemplate(discovery.Namespace, execCtx.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to render namespace template: %w", err)
	}

	// Discover by name
	if discovery.ByName != "" {
		name, err := utils.RenderTemplate(discovery.ByName, execCtx.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to render byName template: %w", err)
		}

		// For maestro: use ManifestWork GVK
		// For k8s: parse the rendered manifest to get GVK
		gvk := re.resolveGVK(resource)

		return re.client.GetResource(ctx, gvk, namespace, name, transportTarget)
	}

	// Discover by label selector
	if discovery.BySelectors != nil && len(discovery.BySelectors.LabelSelector) > 0 {
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

		gvk := re.resolveGVK(resource)

		list, err := re.client.DiscoverResources(ctx, gvk, discoveryConfig, transportTarget)
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

// discoverNestedResources discovers sub-resources within a parent resource (e.g., manifests inside a ManifestWork).
// Each nestedDiscovery is matched against the parent's nested manifests using manifest.DiscoverNestedManifest.
func (re *ResourceExecutor) discoverNestedResources(
	ctx context.Context,
	resource configloader.Resource,
	execCtx *ExecutionContext,
	parent *unstructured.Unstructured,
) map[string]*unstructured.Unstructured {
	nestedResults := make(map[string]*unstructured.Unstructured)

	for _, nd := range resource.NestedDiscoveries {
		if nd.Discovery == nil {
			continue
		}

		// Build discovery config with rendered templates
		discoveryConfig, err := re.buildNestedDiscoveryConfig(nd.Discovery, execCtx.Params)
		if err != nil {
			re.log.Warnf(ctx, "Resource[%s] nested discovery[%s] failed to build config: %v",
				resource.Name, nd.Name, err)
			continue
		}

		// Search within the parent resource
		list, err := manifest.DiscoverNestedManifest(parent, discoveryConfig)
		if err != nil {
			re.log.Warnf(ctx, "Resource[%s] nested discovery[%s] failed: %v",
				resource.Name, nd.Name, err)
			continue
		}

		if len(list.Items) == 0 {
			re.log.Debugf(ctx, "Resource[%s] nested discovery[%s] found no matches",
				resource.Name, nd.Name)
			continue
		}

		// Use the latest generation match
		best := manifest.GetLatestGenerationFromList(list)
		if best != nil {
			manifest.EnrichWithResourceStatus(parent, best)
			nestedResults[nd.Name] = best
			re.log.Debugf(ctx, "Resource[%s] nested discovery[%s] found: %s/%s",
				resource.Name, nd.Name, best.GetKind(), best.GetName())
		}
	}

	return nestedResults
}

// buildNestedDiscoveryConfig renders templates in a discovery config and returns a manifest.DiscoveryConfig.
func (re *ResourceExecutor) buildNestedDiscoveryConfig(
	discovery *configloader.DiscoveryConfig,
	params map[string]interface{},
) (*manifest.DiscoveryConfig, error) {
	namespace, err := utils.RenderTemplate(discovery.Namespace, params)
	if err != nil {
		return nil, fmt.Errorf("failed to render namespace template: %w", err)
	}

	if discovery.ByName != "" {
		name, err := utils.RenderTemplate(discovery.ByName, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render byName template: %w", err)
		}
		return &manifest.DiscoveryConfig{
			Namespace: namespace,
			ByName:    name,
		}, nil
	}

	if discovery.BySelectors != nil && len(discovery.BySelectors.LabelSelector) > 0 {
		renderedLabels := make(map[string]string)
		for k, v := range discovery.BySelectors.LabelSelector {
			renderedK, err := utils.RenderTemplate(k, params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label key template: %w", err)
			}
			renderedV, err := utils.RenderTemplate(v, params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label value template: %w", err)
			}
			renderedLabels[renderedK] = renderedV
		}
		return &manifest.DiscoveryConfig{
			Namespace:     namespace,
			LabelSelector: manifest.BuildLabelSelector(renderedLabels),
		}, nil
	}

	return nil, fmt.Errorf("discovery must specify byName or bySelectors")
}

// resolveGVK extracts the GVK from the resource's manifest.
// Works for both K8s resources and ManifestWorks since both have apiVersion and kind.
func (re *ResourceExecutor) resolveGVK(resource configloader.Resource) schema.GroupVersionKind {
	var manifestData map[string]interface{}

	switch m := resource.Manifest.(type) {
	case map[string]interface{}:
		manifestData = m
	case string:
		// String manifests may contain Go template directives ({{ if }}, {{ range }})
		// that make them invalid YAML. Extract apiVersion and kind by scanning lines
		// instead of full YAML parsing.
		return manifest.ExtractGVKFromString(m)
	default:
		return schema.GroupVersionKind{}
	}

	apiVersion, ok1 := manifestData["apiVersion"].(string)
	kind, ok2 := manifestData["kind"].(string)
	if !ok1 || !ok2 {
		return schema.GroupVersionKind{}
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionKind{}
	}
	return gv.WithKind(kind)
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
