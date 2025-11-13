package k8sclient

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// ResourceTemplate represents a Kubernetes resource template
type ResourceTemplate struct {
	// Template is the YAML template content
	Template string
	// Track contains tracking configuration for this resource
	Track *TrackConfig
}

// TrackConfig defines how to track a resource
type TrackConfig struct {
	// As is the alias name for referencing this resource in expressions
	As string
	// Discovery defines how to discover/find this resource
	Discovery DiscoveryConfig
}

// DiscoveryConfig defines resource discovery rules
type DiscoveryConfig struct {
	// Namespace is the namespace where the resource exists (supports template variables)
	// If empty, uses the namespace from the resource spec or default namespace
	Namespace string
	// ByName contains direct name-based lookup configuration
	ByName *DiscoveryByName
	// BySelectors contains label selector-based discovery configuration
	BySelectors *DiscoveryBySelectors
}

// DiscoveryByName defines direct name lookup
type DiscoveryByName struct {
	// Name is the resource name (supports template variables)
	Name string
}

// DiscoveryBySelectors defines label selector-based discovery
type DiscoveryBySelectors struct {
	// LabelSelector is a map of label key-value pairs (supports template variables)
	LabelSelector map[string]string
}

// RenderTemplate renders a resource template with the given variables
func RenderTemplate(tmpl string, variables map[string]interface{}) (string, error) {
	// Create template with sprig functions
	t, err := template.New("resource").Funcs(sprig.TxtFuncMap()).Parse(tmpl)
	if err != nil {
		return "", errors.KubernetesError("failed to parse template: %v", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := t.Execute(&buf, variables); err != nil {
		return "", errors.KubernetesError("failed to execute template: %v", err)
	}

	return buf.String(), nil
}

// ParseYAMLToUnstructured parses YAML content into an unstructured object
func ParseYAMLToUnstructured(yamlContent string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(yamlContent)), 4096)
	if err := decoder.Decode(obj); err != nil {
		return nil, errors.KubernetesError("failed to decode YAML: %v", err)
	}

	return obj, nil
}

// RenderAndParseResource renders a template and parses it into an unstructured object
func RenderAndParseResource(tmpl string, variables map[string]interface{}) (*unstructured.Unstructured, error) {
	// Render template
	rendered, err := RenderTemplate(tmpl, variables)
	if err != nil {
		return nil, errors.KubernetesError("failed to render template: %v", err)
	}

	// Parse YAML to unstructured
	obj, err := ParseYAMLToUnstructured(rendered)
	if err != nil {
		return nil, errors.KubernetesError("failed to parse rendered template: %v", err)
	}

	return obj, nil
}

// RenderDiscoveryName renders a discovery name template with variables
func RenderDiscoveryName(nameTemplate string, variables map[string]interface{}) (string, error) {
	return RenderTemplate(nameTemplate, variables)
}

// RenderDiscoverySelectors renders discovery label selectors with variables
func RenderDiscoverySelectors(selectors map[string]string, variables map[string]interface{}) (map[string]string, error) {
	rendered := make(map[string]string)
	
	for key, valueTemplate := range selectors {
		// Render the key
		renderedKey, err := RenderTemplate(key, variables)
		if err != nil {
			return nil, errors.KubernetesError("failed to render selector key %s: %v", key, err)
		}

		// Render the value
		renderedValue, err := RenderTemplate(valueTemplate, variables)
		if err != nil {
			return nil, errors.KubernetesError("failed to render selector value for key %s: %v", key, err)
		}

		rendered[renderedKey] = renderedValue
	}

	return rendered, nil
}

// BuildLabelSelector converts a map of labels to a label selector string
func BuildLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	var selector string
	first := true
	for key, value := range labels {
		if !first {
			selector += ","
		}
		selector += fmt.Sprintf("%s=%s", key, value)
		first = false
	}
	return selector
}

