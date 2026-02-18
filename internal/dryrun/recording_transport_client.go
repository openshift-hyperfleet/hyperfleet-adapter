package dryrun

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transport_client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TransportRecord stores details of a transport client operation.
type TransportRecord struct {
	Operation string // "apply", "get", "discover"
	GVK       schema.GroupVersionKind
	Namespace string
	Name      string
	Manifest  []byte
	Result    *transport_client.ApplyResult
	Error     error
}

// DryrunTransportClient implements transport_client.TransportClient by recording
// all operations in-memory without executing real Kubernetes calls.
// Applied resources are stored for subsequent discovery/get operations.
type DryrunTransportClient struct {
	mu                 sync.Mutex
	resources          map[string]*unstructured.Unstructured // key: "namespace/name/gvk"
	Records            []TransportRecord
	discoveryOverrides DiscoveryOverrides
}

// NewDryrunTransportClient creates a new DryrunTransportClient.
func NewDryrunTransportClient() *DryrunTransportClient {
	return &DryrunTransportClient{
		resources: make(map[string]*unstructured.Unstructured),
		Records:   make([]TransportRecord, 0),
	}
}

// NewDryrunTransportClientWithOverrides creates a DryrunTransportClient
// with discovery overrides. When a resource is applied and its metadata.name
// matches a key in the overrides map, the override object replaces the applied
// manifest in the in-memory store.
func NewDryrunTransportClientWithOverrides(overrides DiscoveryOverrides) *DryrunTransportClient {
	return &DryrunTransportClient{
		resources:          make(map[string]*unstructured.Unstructured),
		Records:            make([]TransportRecord, 0),
		discoveryOverrides: overrides,
	}
}

func resourceKey(gvk schema.GroupVersionKind, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, namespace, name)
}

// ApplyResource parses the manifest JSON, stores it in-memory, and records the operation.
func (c *DryrunTransportClient) ApplyResource(ctx context.Context, manifestBytes []byte, opts *transport_client.ApplyOptions, target transport_client.TransportContext) (*transport_client.ApplyResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Parse manifest
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(manifestBytes, &obj.Object); err != nil {
		record := TransportRecord{
			Operation: "apply",
			Manifest:  manifestBytes,
			Error:     fmt.Errorf("failed to parse manifest: %w", err),
		}
		c.Records = append(c.Records, record)
		return nil, record.Error
	}

	gvk := obj.GroupVersionKind()
	namespace := obj.GetNamespace()
	name := obj.GetName()
	key := resourceKey(gvk, namespace, name)

	// Determine operation: create or update
	var operation manifest.Operation
	if _, exists := c.resources[key]; exists {
		operation = manifest.OperationUpdate
	} else {
		operation = manifest.OperationCreate
	}

	if opts != nil && opts.RecreateOnChange && operation == manifest.OperationUpdate {
		operation = manifest.OperationRecreate
	}

	// Check for discovery override by resource name
	if c.discoveryOverrides != nil {
		if override, found := c.discoveryOverrides[name]; found {
			overrideObj := &unstructured.Unstructured{Object: override}
			c.resources[key] = overrideObj
		} else {
			c.resources[key] = obj
		}
	} else {
		c.resources[key] = obj
	}

	result := &transport_client.ApplyResult{
		Operation: operation,
		Reason:    fmt.Sprintf("dry-run %s", operation),
	}

	c.Records = append(c.Records, TransportRecord{
		Operation: "apply",
		GVK:       gvk,
		Namespace: namespace,
		Name:      name,
		Manifest:  manifestBytes,
		Result:    result,
	})

	return result, nil
}

// GetResource returns a resource from the in-memory store or a NotFound error.
func (c *DryrunTransportClient) GetResource(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, target transport_client.TransportContext) (*unstructured.Unstructured, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := resourceKey(gvk, namespace, name)
	obj, exists := c.resources[key]

	c.Records = append(c.Records, TransportRecord{
		Operation: "get",
		GVK:       gvk,
		Namespace: namespace,
		Name:      name,
	})

	if !exists {
		return nil, fmt.Errorf("resource %s/%s %s/%s not found (dry-run)", gvk.Kind, gvk.Version, namespace, name)
	}

	return obj.DeepCopy(), nil
}

// DiscoverResources returns resources from the in-memory store filtered by discovery config.
func (c *DryrunTransportClient) DiscoverResources(ctx context.Context, gvk schema.GroupVersionKind, discovery manifest.Discovery, target transport_client.TransportContext) (*unstructured.UnstructuredList, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Records = append(c.Records, TransportRecord{
		Operation: "discover",
		GVK:       gvk,
		Namespace: discovery.GetNamespace(),
		Name:      discovery.GetName(),
	})

	list := &unstructured.UnstructuredList{}

	for _, obj := range c.resources {
		objGVK := obj.GroupVersionKind()
		if objGVK.Group != gvk.Group || objGVK.Version != gvk.Version || objGVK.Kind != gvk.Kind {
			continue
		}

		// Filter by namespace
		ns := discovery.GetNamespace()
		if ns != "" && ns != "*" && obj.GetNamespace() != ns {
			continue
		}

		// Filter by name if single-resource discovery
		if discovery.IsSingleResource() && obj.GetName() != discovery.GetName() {
			continue
		}

		// Filter by label selector if provided
		if !discovery.IsSingleResource() && discovery.GetLabelSelector() != "" {
			if !manifest.MatchesLabels(obj, discovery.GetLabelSelector()) {
				continue
			}
		}

		list.Items = append(list.Items, *obj.DeepCopy())
	}

	return list, nil
}
