package k8s_client

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	apperrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourcePath represents a parsed Kubernetes resource path
type ResourcePath struct {
	Namespace    string
	ResourceName string
	Key          string
}

// ParseResourcePath parses a path in the format: namespace.name.key
func ParseResourcePath(path, resourceType string) (*ResourcePath, error) {
	parts := strings.Split(path, ".")
	if len(parts) < 3 {
		return nil, apperrors.NewK8sInvalidPathError(resourceType, path, "namespace.name.key")
	}

	return &ResourcePath{
		Namespace:    parts[0],
		ResourceName: parts[1],
		Key:          strings.Join(parts[2:], "."), // Allow dots in key name
	}, nil
}

// GetResourceData retrieves data from a Kubernetes resource (Secret or ConfigMap)
func (c *Client) GetResourceData(ctx context.Context, gvk schema.GroupVersionKind, namespace, name, resourceType string) (map[string]interface{}, error) {
	resource, err := c.GetResource(ctx, gvk, namespace, name)
	if err != nil {
		return nil, apperrors.NewK8sResourceDataError(resourceType, namespace, name, "failed to get resource", err)
	}

	data, found, err := unstructured.NestedMap(resource.Object, "data")
	if err != nil {
		return nil, apperrors.NewK8sResourceDataError(resourceType, namespace, name, "failed to access data field", err)
	}
	if !found {
		return nil, apperrors.NewK8sResourceDataError(resourceType, namespace, name, "no data field found", nil)
	}

	return data, nil
}

// ExtractFromSecret extracts a value from a Kubernetes Secret
// Format: namespace.name.key (namespace is required)
func (c *Client) ExtractFromSecret(ctx context.Context, path string) (string, error) {
	resourcePath, err := ParseResourcePath(path, "secret")
	if err != nil {
		return "", err
	}

	secretGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	data, err := c.GetResourceData(ctx, secretGVK, resourcePath.Namespace, resourcePath.ResourceName, "Secret")
	if err != nil {
		return "", err
	}

	encodedValue, ok := data[resourcePath.Key]
	if !ok {
		return "", apperrors.NewK8sResourceKeyNotFoundError("Secret", resourcePath.Namespace, resourcePath.ResourceName, resourcePath.Key)
	}

	encodedStr, ok := encodedValue.(string)
	if !ok {
		return "", apperrors.NewK8sResourceDataError("Secret", resourcePath.Namespace, resourcePath.ResourceName,
			fmt.Sprintf("data for key '%s' is not a string", resourcePath.Key), nil)
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(encodedStr)
	if err != nil {
		return "", apperrors.NewK8sResourceDataError("Secret", resourcePath.Namespace, resourcePath.ResourceName,
			fmt.Sprintf("failed to decode data for key '%s'", resourcePath.Key), err)
	}

	return string(decodedBytes), nil
}

// ExtractFromConfigMap extracts a value from a Kubernetes ConfigMap
// Format: namespace.name.key (namespace is required)
func (c *Client) ExtractFromConfigMap(ctx context.Context, path string) (string, error) {
	resourcePath, err := ParseResourcePath(path, "configmap")
	if err != nil {
		return "", err
	}

	configMapGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	data, err := c.GetResourceData(ctx, configMapGVK, resourcePath.Namespace, resourcePath.ResourceName, "ConfigMap")
	if err != nil {
		return "", err
	}

	value, ok := data[resourcePath.Key]
	if !ok {
		return "", apperrors.NewK8sResourceKeyNotFoundError("ConfigMap", resourcePath.Namespace, resourcePath.ResourceName, resourcePath.Key)
	}

	valueStr, ok := value.(string)
	if !ok {
		return "", apperrors.NewK8sResourceDataError("ConfigMap", resourcePath.Namespace, resourcePath.ResourceName,
			fmt.Sprintf("data for key '%s' is not a string", resourcePath.Key), nil)
	}

	return valueStr, nil
}
