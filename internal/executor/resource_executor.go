package executor

import (
	"context"
	"fmt"

	"github.com/mitchellh/copystructure"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/config_loader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/constants"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// ResourceExecutor creates and updates Kubernetes resources
type ResourceExecutor struct {
	client transport_client.TransportClient
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
func (re *ResourceExecutor) ExecuteAll(ctx context.Context, resources []config_loader.Resource, execCtx *ExecutionContext) ([]ResourceResult, error) {
	if execCtx.Resources == nil {
		execCtx.Resources = make(map[string]*unstructured.Unstructured)
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

// executeResource creates or updates a single resource.
// The executor no longer needs to know which transport it's talking to.
// For K8s transport: builds a single manifest, discovers existing, calls ApplyResources.
// For Maestro transport: builds multiple manifests, populates TransportConfig in ApplyOptions,
// calls the same ApplyResources — the Maestro client internally builds the ManifestWork.
func (re *ResourceExecutor) executeResource(ctx context.Context, resource config_loader.Resource, execCtx *ExecutionContext) (ResourceResult, error) {
	result := ResourceResult{
		Name:   resource.Name,
		Status: StatusSuccess,
	}

	if re.client == nil {
		result.Status = StatusFailed
		result.Error = fmt.Errorf("transport client not configured")
		return result, NewExecutorError(PhaseResources, resource.Name, "transport client not configured", result.Error)
	}

	clientType := resource.Transport.GetClientType()

	var manifests []*unstructured.Unstructured

	switch clientType {
	case config_loader.TransportClientKubernetes:
		// Build single manifest for Kubernetes transport
		re.log.Debugf(ctx, "Building manifest from config for Kubernetes transport")
		m, err := re.buildManifestK8s(ctx, resource, execCtx)
		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			return result, NewExecutorError(PhaseResources, resource.Name, "failed to build manifest", err)
		}
		manifests = []*unstructured.Unstructured{m}

	case config_loader.TransportClientMaestro:
		// Build multiple manifests for Maestro transport
		re.log.Debugf(ctx, "Building manifests from config for Maestro transport")
		built, err := re.buildManifestsMaestro(ctx, resource, execCtx)
		if err != nil {
			result.Status = StatusFailed
			result.Error = err
			return result, NewExecutorError(PhaseResources, resource.Name, "failed to build manifests", err)
		}
		manifests = built
		re.log.Debugf(ctx, "Resource[%s] built %d manifests for ManifestWork", resource.Name, len(manifests))

	default:
		result.Status = StatusFailed
		result.Error = fmt.Errorf("unsupported transport client: %s", clientType)
		return result, NewExecutorError(PhaseResources, resource.Name, "unsupported transport client", result.Error)
	}

	// Set result info from first manifest
	if len(manifests) > 0 {
		firstManifest := manifests[0]
		gvk := firstManifest.GroupVersionKind()
		result.Kind = gvk.Kind
		result.Namespace = firstManifest.GetNamespace()
		result.ResourceName = firstManifest.GetName()

		// Add K8s resource context fields for logging
		ctx = logger.WithK8sKind(ctx, result.Kind)
		ctx = logger.WithK8sName(ctx, result.ResourceName)
		ctx = logger.WithK8sNamespace(ctx, result.Namespace)
	}

	// Add observed_generation to context
	if len(manifests) > 0 {
		manifestGen := manifest.GetGenerationFromUnstructured(manifests[0])
		ctx = logger.WithObservedGeneration(ctx, manifestGen)
	}

	// Build resources to apply with discovery
	resourcesToApply := make([]transport_client.ResourceToApply, 0, len(manifests))

	if clientType == config_loader.TransportClientKubernetes {
		// For K8s: discover existing resource for generation comparison
		m := manifests[0]
		gvk := m.GroupVersionKind()

		var existingResource *unstructured.Unstructured
		var err error
		if resource.Discovery != nil {
			re.log.Debugf(ctx, "Discovering existing resource using discovery config...")
			existingResource, err = re.discoverExistingResource(ctx, gvk, resource.Discovery, execCtx)
		} else {
			re.log.Debugf(ctx, "Looking up existing resource by name...")
			existingResource, err = re.client.GetResource(ctx, gvk, m.GetNamespace(), m.GetName())
		}

		if err != nil && !apierrors.IsNotFound(err) {
			result.Status = StatusFailed
			result.Error = err
			return result, NewExecutorError(PhaseResources, resource.Name, "failed to find existing resource", err)
		}

		if existingResource != nil {
			re.log.Debugf(ctx, "Existing resource found: %s/%s", existingResource.GetNamespace(), existingResource.GetName())
		} else {
			re.log.Debugf(ctx, "No existing resource found, will create")
		}

		resourcesToApply = append(resourcesToApply, transport_client.ResourceToApply{
			Manifest:         m,
			ExistingResource: existingResource,
		})
	} else {
		// For Maestro: no individual resource discovery, just bundle all manifests
		for _, m := range manifests {
			resourcesToApply = append(resourcesToApply, transport_client.ResourceToApply{
				Manifest:         m,
				ExistingResource: nil,
			})
		}
	}

	// Build ApplyOptions
	opts := transport_client.ApplyOptions{
		RecreateOnChange: resource.RecreateOnChange,
	}

	// For Maestro transport, populate TransportConfig
	if clientType == config_loader.TransportClientMaestro {
		if resource.Transport == nil || resource.Transport.Maestro == nil {
			result.Status = StatusFailed
			result.Error = fmt.Errorf("maestro transport configuration missing")
			return result, NewExecutorError(PhaseResources, resource.Name, "maestro transport configuration missing", result.Error)
		}

		maestroConfig := resource.Transport.Maestro

		// Render targetCluster template
		targetCluster, err := renderTemplate(maestroConfig.TargetCluster, execCtx.Params)
		if err != nil {
			result.Status = StatusFailed
			result.Error = fmt.Errorf("failed to render targetCluster template: %w", err)
			return result, NewExecutorError(PhaseResources, resource.Name, "failed to render targetCluster template", err)
		}

		re.log.Debugf(ctx, "Resource[%s] using Maestro transport to cluster=%s with %d manifests",
			resource.Name, targetCluster, len(manifests))

		tc := map[string]interface{}{
			"targetCluster": targetCluster,
			"resourceName":  resource.Name,
			"params":        execCtx.Params,
		}

		// Render manifestWork name if configured
		if maestroConfig.ManifestWork != nil && maestroConfig.ManifestWork.Name != "" {
			workName, err := renderTemplate(maestroConfig.ManifestWork.Name, execCtx.Params)
			if err != nil {
				result.Status = StatusFailed
				result.Error = fmt.Errorf("failed to render manifestWork.name template: %w", err)
				return result, NewExecutorError(PhaseResources, resource.Name, "failed to render manifestWork.name template", err)
			}
			tc["manifestWorkName"] = workName
		}

		// Pass refContent if present
		if maestroConfig.ManifestWork != nil && maestroConfig.ManifestWork.RefContent != nil {
			tc["manifestWorkRefContent"] = maestroConfig.ManifestWork.RefContent
		}

		opts.TransportConfig = tc
	}

	// Call ApplyResources uniformly
	applyResults, err := re.client.ApplyResources(ctx, resourcesToApply, opts)
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
		re.log.Errorf(errCtx, "Resource[%s] apply failed", resource.Name)
		// Log manifests for debugging
		for i, m := range manifests {
			if manifestYAML, marshalErr := yaml.Marshal(m.Object); marshalErr == nil {
				re.log.Debugf(errCtx, "Resource[%s] failed manifest[%d]:\n%s", resource.Name, i, string(manifestYAML))
			}
		}
		return result, NewExecutorError(PhaseResources, resource.Name, "failed to apply resource", err)
	}

	// Check for per-resource errors
	if applyResults != nil && len(applyResults.Results) > 0 && applyResults.Results[0].Error != nil {
		applyErr := applyResults.Results[0].Error
		result.Status = StatusFailed
		result.Error = applyErr
		result.Operation = manifest.Operation(applyResults.Results[0].Operation)
		result.OperationReason = applyResults.Results[0].Reason
		execCtx.Adapter.ExecutionError = &ExecutionError{
			Phase:   string(PhaseResources),
			Step:    resource.Name,
			Message: applyErr.Error(),
		}
		errCtx := logger.WithK8sResult(ctx, "FAILED")
		errCtx = logger.WithErrorField(errCtx, applyErr)
		re.log.Errorf(errCtx, "Resource[%s] processed: operation=%s reason=%s",
			resource.Name, result.Operation, result.OperationReason)
		if manifestYAML, marshalErr := yaml.Marshal(manifests[0].Object); marshalErr == nil {
			re.log.Debugf(errCtx, "Resource[%s] failed manifest:\n%s", resource.Name, string(manifestYAML))
		}
		return result, NewExecutorError(PhaseResources, resource.Name,
			fmt.Sprintf("failed to %s resource", result.Operation), applyErr)
	}

	// Extract result from ApplyResources
	if applyResults != nil && len(applyResults.Results) > 0 {
		applyResult := applyResults.Results[0]
		result.Operation = manifest.Operation(applyResult.Operation)
		result.OperationReason = applyResult.Reason
		result.Resource = applyResult.Resource
	}

	successCtx := logger.WithK8sResult(ctx, "SUCCESS")
	re.log.Infof(successCtx, "Resource[%s] processed: operation=%s reason=%s",
		resource.Name, result.Operation, result.OperationReason)

	// Store resources in execution context
	if clientType == config_loader.TransportClientMaestro {
		// Store each manifest by compound name (resource.manifestName)
		for i, m := range manifests {
			if i < len(resource.Manifests) {
				manifestName := resource.Manifests[i].Name
				key := resource.Name + "." + manifestName
				execCtx.Resources[key] = m
				re.log.Debugf(ctx, "Resource stored in context as '%s'", key)
			}
		}
		// Store first manifest under resource name for convenience
		if len(manifests) > 0 {
			execCtx.Resources[resource.Name] = manifests[0]
			re.log.Debugf(ctx, "First manifest also stored in context as '%s'", resource.Name)
		}
	} else {
		// K8s transport: store single resource
		if result.Resource != nil {
			execCtx.Resources[resource.Name] = result.Resource
			re.log.Debugf(ctx, "Resource stored in context as '%s'", resource.Name)
		}
	}

	return result, nil
}

// buildManifestK8s builds an unstructured manifest from the resource configuration for Kubernetes transport
func (re *ResourceExecutor) buildManifestK8s(ctx context.Context, resource config_loader.Resource, execCtx *ExecutionContext) (*unstructured.Unstructured, error) {
	var manifestData map[string]interface{}

	// Get manifest (inline or loaded from ref)
	if resource.Manifest != nil {
		switch m := resource.Manifest.(type) {
		case map[string]interface{}:
			manifestData = m
		case map[interface{}]interface{}:
			manifestData = convertToStringKeyMap(m)
		default:
			return nil, fmt.Errorf("unsupported manifest type: %T", resource.Manifest)
		}
	} else {
		return nil, fmt.Errorf("no manifest specified for resource %s", resource.Name)
	}

	// Deep copy to avoid modifying the original
	manifestData = deepCopyMap(ctx, manifestData, re.log)

	// Render all template strings in the manifest
	renderedData, err := renderManifestTemplates(manifestData, execCtx.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to render manifest templates: %w", err)
	}

	// Convert to unstructured
	obj := &unstructured.Unstructured{Object: renderedData}

	// Validate manifest
	if err := validateManifest(obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// buildManifestsMaestro builds unstructured manifests from the resource.Manifests array for Maestro transport
func (re *ResourceExecutor) buildManifestsMaestro(ctx context.Context, resource config_loader.Resource, execCtx *ExecutionContext) ([]*unstructured.Unstructured, error) {
	results := make([]*unstructured.Unstructured, 0, len(resource.Manifests))

	for i, nm := range resource.Manifests {
		content := nm.GetManifestContent()
		if content == nil {
			return nil, fmt.Errorf("manifest[%d] (%s) has no content", i, nm.Name)
		}

		var manifestData map[string]interface{}
		switch m := content.(type) {
		case map[string]interface{}:
			manifestData = m
		case map[interface{}]interface{}:
			manifestData = convertToStringKeyMap(m)
		default:
			return nil, fmt.Errorf("manifest[%d] (%s): unsupported manifest type: %T", i, nm.Name, content)
		}

		// Deep copy to avoid modifying the original
		manifestData = deepCopyMap(ctx, manifestData, re.log)

		// Render all template strings in the manifest
		renderedData, err := renderManifestTemplates(manifestData, execCtx.Params)
		if err != nil {
			return nil, fmt.Errorf("manifest[%d] (%s): failed to render templates: %w", i, nm.Name, err)
		}

		// Convert to unstructured
		obj := &unstructured.Unstructured{Object: renderedData}

		// Validate manifest
		if err := validateManifest(obj); err != nil {
			return nil, fmt.Errorf("manifest[%d] (%s): %w", i, nm.Name, err)
		}

		results = append(results, obj)
	}

	return results, nil
}

// validateManifest validates a Kubernetes manifest has all required fields and annotations
func validateManifest(obj *unstructured.Unstructured) error {
	// Validate required Kubernetes fields
	if obj.GetAPIVersion() == "" {
		return fmt.Errorf("manifest missing apiVersion")
	}
	if obj.GetKind() == "" {
		return fmt.Errorf("manifest missing kind")
	}
	if obj.GetName() == "" {
		return fmt.Errorf("manifest missing metadata.name")
	}

	// Validate required generation annotation
	if manifest.GetGenerationFromUnstructured(obj) == 0 {
		return fmt.Errorf("manifest missing required annotation %q", constants.AnnotationGeneration)
	}

	return nil
}

// discoverExistingResource discovers an existing resource using the discovery config
func (re *ResourceExecutor) discoverExistingResource(ctx context.Context, gvk schema.GroupVersionKind, discovery *config_loader.DiscoveryConfig, execCtx *ExecutionContext) (*unstructured.Unstructured, error) {
	if re.client == nil {
		return nil, fmt.Errorf("transport client not configured")
	}

	// Render discovery namespace template
	// Empty namespace means all namespaces (normalized from "*" at config load time)
	namespace, err := renderTemplate(discovery.Namespace, execCtx.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to render namespace template: %w", err)
	}

	// Check if discovering by name
	if discovery.ByName != "" {
		name, err := renderTemplate(discovery.ByName, execCtx.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to render byName template: %w", err)
		}
		return re.client.GetResource(ctx, gvk, namespace, name)
	}

	// Discover by label selector
	if discovery.BySelectors != nil && len(discovery.BySelectors.LabelSelector) > 0 {
		// Render label selector templates
		renderedLabels := make(map[string]string)
		for k, v := range discovery.BySelectors.LabelSelector {
			renderedK, err := renderTemplate(k, execCtx.Params)
			if err != nil {
				return nil, fmt.Errorf("failed to render label key template: %w", err)
			}
			renderedV, err := renderTemplate(v, execCtx.Params)
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

		list, err := re.client.DiscoverResources(ctx, gvk, discoveryConfig)
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

// convertToStringKeyMap converts map[interface{}]interface{} to map[string]interface{}
func convertToStringKeyMap(m map[interface{}]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		strKey := fmt.Sprintf("%v", k)
		switch val := v.(type) {
		case map[interface{}]interface{}:
			result[strKey] = convertToStringKeyMap(val)
		case []interface{}:
			result[strKey] = convertSlice(val)
		default:
			result[strKey] = v
		}
	}
	return result
}

// convertSlice converts slice elements recursively
func convertSlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[interface{}]interface{}:
			result[i] = convertToStringKeyMap(val)
		case []interface{}:
			result[i] = convertSlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// deepCopyMap creates a deep copy of a map using github.com/mitchellh/copystructure.
// This handles non-JSON-serializable types (channels, functions, time.Time, etc.)
// and preserves type information (e.g., int64 stays int64, not float64).
// If deep copy fails, it falls back to a shallow copy and logs a warning.
// WARNING: Shallow copy means nested maps/slices will share references with the original,
// which could lead to unexpected mutations.
func deepCopyMap(ctx context.Context, m map[string]interface{}, log logger.Logger) map[string]interface{} {
	if m == nil {
		return nil
	}

	copied, err := copystructure.Copy(m)
	if err != nil {
		// Fallback to shallow copy if deep copy fails
		log.Warnf(ctx, "Failed to deep copy map: %v. Falling back to shallow copy.", err)
		result := make(map[string]interface{})
		for k, v := range m {
			result[k] = v
		}
		return result
	}

	result, ok := copied.(map[string]interface{})
	if !ok {
		// Should not happen, but handle gracefully
		result := make(map[string]interface{})
		for k, v := range m {
			result[k] = v
		}
		return result
	}

	return result
}

// renderManifestTemplates recursively renders all template strings in a manifest
func renderManifestTemplates(data map[string]interface{}, params map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for k, v := range data {
		renderedKey, err := renderTemplate(k, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render key '%s': %w", k, err)
		}

		renderedValue, err := renderValue(v, params)
		if err != nil {
			return nil, fmt.Errorf("failed to render value for key '%s': %w", k, err)
		}

		result[renderedKey] = renderedValue
	}

	return result, nil
}

// renderValue renders a value recursively
func renderValue(v interface{}, params map[string]interface{}) (interface{}, error) {
	switch val := v.(type) {
	case string:
		return renderTemplate(val, params)
	case map[string]interface{}:
		return renderManifestTemplates(val, params)
	case map[interface{}]interface{}:
		converted := convertToStringKeyMap(val)
		return renderManifestTemplates(converted, params)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			rendered, err := renderValue(item, params)
			if err != nil {
				return nil, err
			}
			result[i] = rendered
		}
		return result, nil
	default:
		return v, nil
	}
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
