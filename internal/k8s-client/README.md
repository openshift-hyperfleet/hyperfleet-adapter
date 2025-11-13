# Kubernetes Client Package (`k8sclient`)

This package provides a comprehensive Kubernetes client implementation for the HyperFleet adapter framework.

**Package:** `github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8s-client`

## Overview

The k8s-client package enables adapters to:
- Create and manage Kubernetes resources dynamically
- Use Go templates with Sprig functions for resource definitions
- Track resources for status evaluation
- Discover resources by name or label selectors
- Extract status information for reporting

## Architecture

```
┌─────────────────────────────────────────────────┐
│           ResourceManager                        │
│  (High-level orchestration)                     │
│                                                  │
│  • CreateResourceFromTemplate()                 │
│  • CreateOrUpdateResource()                     │
│  • ResourceExists()                             │
│  • DiscoverAndTrack()                           │
└──────────────┬────────────────┬─────────────────┘
               │                │
       ┌───────▼────────┐  ┌───▼────────────────┐
       │    Client      │  │  ResourceTracker   │
       │                │  │                    │
       │  • Create      │  │  • Track           │
       │  • Get         │  │  • Discover        │
       │  • List        │  │  • Refresh         │
       │  • Update      │  │  • Extract Status  │
       │  • Delete      │  │  • Build Variables │
       │  • Patch       │  │                    │
       └────────────────┘  └────────────────────┘
```

## Components

### 1. Client (`client.go`)

Low-level Kubernetes API client using dynamic client and REST mapper.

**Key Features:**
- Works with any Kubernetes resource type (including CRDs)
- Automatic GVK to GVR conversion
- Support for both namespaced and cluster-scoped resources
- In-cluster and kubeconfig-based authentication

**Authentication:**

The client supports two authentication methods with automatic detection:

1. **ServiceAccount (Production)** - When `KubeConfigPath` is empty `""`
   - Automatically uses the pod's ServiceAccount token
   - Token mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`
   - Perfect for production deployments in Kubernetes clusters
   - Requires proper RBAC permissions

2. **Kubeconfig (Development)** - When `KubeConfigPath` is set
   - Uses specified kubeconfig file for authentication
   - Ideal for local development and testing
   - Can access remote clusters

<details>
<summary>Click to expand: Client Authentication Examples</summary>

```go
// Production deployment in K8s cluster (uses ServiceAccount)
config := ClientConfig{
    KubeConfigPath: "", // empty = use ServiceAccount automatically
    QPS: 100.0,
    Burst: 200,
}
client, err := NewClient(ctx, config, log)
// Client logs: "Using in-cluster Kubernetes configuration (ServiceAccount)"

// Local development (uses kubeconfig)
config := ClientConfig{
    KubeConfigPath: "/home/user/.kube/config",
    QPS: 50.0,
    Burst: 100,
}
client, err := NewClient(ctx, config, log)
// Client logs: "Using kubeconfig from: /home/user/.kube/config"

// Get a resource (extract GVK from config)
gvk, err := GVKFromKindAndApiVersion("Deployment", "apps/v1")
resource, err := client.GetResource(ctx, gvk, "default", "my-deployment")

// List resources by label
list, err := client.ListResources(ctx, gvk, "default", "app=myapp,env=prod")
```

</details>

**Note:** In production, always extract GVK from your adapter config using `GVKFromKindAndApiVersion()`.

### 2. Template Processing (`template.go`)

Renders Go templates with Sprig functions and parses them into Kubernetes resources.

**Supported Functions:**
- All Sprig v3 functions (strings, dates, crypto, etc.)
- Standard Go template functions

<details>
<summary>Click to expand: Template Rendering Example</summary>

```go
// Define template
template := `
apiVersion: v1
kind: Namespace
metadata:
  name: cluster-{{ .clusterId }}
  labels:
    hyperfleet.io/cluster-id: "{{ .clusterId }}"
    hyperfleet.io/region: "{{ .region }}"
`

// Render with variables
variables := map[string]interface{}{
    "clusterId": "abc123",
    "region": "us-east-1",
}

obj, err := RenderAndParseResource(template, variables)
// Creates Namespace with name "cluster-abc123"
```

</details>

### 3. Resource Tracker (`tracker.go`)

Tracks Kubernetes resources for status evaluation and expression evaluation.

**Key Features:**
- Track resources by alias
- Discover resources by name or label selectors
- Refresh resource status from API
- Extract status fields and nested data
- Build variables map for expression evaluation

**Architecture Note:** The tracker validates inputs and stores resources, but does NOT render templates. Template rendering is handled by the `ResourceManager` which then calls tracker with already-rendered values.

<details>
<summary>Click to expand: Tracker Usage Examples</summary>

**Via Manager (Recommended):**
```go
// Create manager (manager handles rendering and calls tracker)
manager := NewResourceManager(client, log)

// Discover and track with templated namespace and name
trackConfig := TrackConfig{
    As: "provisionerJob",
    Discovery: DiscoveryConfig{
        Namespace: "cluster-{{ .clusterId }}-ns",  // Manager renders this
        ByName: &DiscoveryByName{
            Name: "cluster-provisioner-{{ .clusterId }}",  // Manager renders this
        },
    },
}

variables := map[string]interface{}{
    "clusterId": "abc123",
}

gvk, _ := GVKFromKindAndApiVersion("Job", "batch/v1")
err := manager.DiscoverAndTrack(ctx, gvk, trackConfig, variables)
// Manager renders templates, then tracker discovers Job in namespace "cluster-abc123-ns"

// Extract status via manager
tracker := manager.GetTracker()
status, err := tracker.ExtractStatus("provisionerJob")

// Build variables for expressions
vars := tracker.BuildVariablesMap()
// vars["resources"]["provisionerJob"] = {...}
```

**Direct Tracker Usage (Testing Only):**
```go
// For testing, you can call tracker directly with already-rendered values
tracker := NewResourceTracker(client, log)
gvk, _ := GVKFromKindAndApiVersion("Job", "batch/v1")
err := tracker.DiscoverAndTrackByName(ctx, "myJob", gvk, "default", "my-job-name")
```

</details>

### 4. Resource Manager (`manager.go`)

High-level manager that orchestrates client and tracker operations.

<details>
<summary>Click to expand: Resource Manager Examples</summary>

```go
// Create manager
manager := NewResourceManager(client, log)

// Create resource from template with tracking
tmpl := ResourceTemplate{
    Template: yamlTemplate,
    Track: &TrackConfig{
        As: "myNamespace",
        Discovery: DiscoveryConfig{
            ByName: &DiscoveryByName{
                Name: "{{ .namespaceName }}",
            },
        },
    },
}

created, err := manager.CreateResourceFromTemplate(ctx, tmpl, variables)

// Check if resource exists (namespace in discovery config)
discovery := DiscoveryConfig{
    Namespace: "cluster-{{ .clusterId }}-ns",
    ByName: &DiscoveryByName{
        Name: "my-resource-{{ .resourceId }}",
    },
}

exists, resource, err := manager.ResourceExists(
    ctx,
    gvk,
    discovery,
    variables,
)

// Get tracked resources as variables
vars := manager.GetTrackedResourcesAsVariables()
```

</details>

## Resource Discovery

Resources can be discovered in two ways, with **namespace support via template variables**:

<details>
<summary>Click to expand: Resource Discovery Methods</summary>

### 1. By Name (Direct Lookup)

```go
discovery := DiscoveryConfig{
    Namespace: "cluster-{{ .clusterId }}-workloads",  // ✨ Namespace supports templates!
    ByName: &DiscoveryByName{
        Name: "cluster-{{ .clusterId }}-job",
    },
}
```

### 2. By Label Selectors

```go
discovery := DiscoveryConfig{
    Namespace: "hyperfleet-system",  // Can be static or templated
    BySelectors: &DiscoveryBySelectors{
        LabelSelector: map[string]string{
            "hyperfleet.io/cluster-id": "{{ .clusterId }}",
            "hyperfleet.io/resource-type": "provisioner",
        },
    },
}
```

</details>

**Namespace Templating:**
- ✅ Supports all template variables (event data, cluster info, etc.)
- ✅ Leave empty (`""`) for cluster-scoped resources (Namespace, ClusterRole, etc.)
- ✅ Use static namespace strings when appropriate
- ✅ Parse namespace from resource metadata dynamically

## Usage in Adapter Framework

The k8s client integrates with the adapter framework workflow:

<details>
<summary>Click to expand: Usage in Adapter Framework</summary>

### 1. Create Resources (Preconditions Met)

```go
// Render and create resources from config templates
for _, resourceConfig := range adapterConfig.Resources {
    tmpl := ResourceTemplate{
        Template: resourceConfig.Template,
        Track: resourceConfig.Track,
    }
    
    _, err := manager.CreateResourceFromTemplate(ctx, tmpl, variables)
    if err != nil {
        log.Error(fmt.Sprintf("Failed to create resource: %v", err))
        return err
    }
}
```

### 2. Discover Tracked Resources (Post-Processing)

```go
// Discover resources for status evaluation
for _, resourceConfig := range adapterConfig.Resources {
    if resourceConfig.Track != nil {
        err := manager.DiscoverAndTrack(
            ctx,
            resourceConfig.GVK,
            resourceConfig.Namespace,
            *resourceConfig.Track,
            variables,
        )
        if err != nil {
            log.Warning(fmt.Sprintf("Failed to discover resource: %v", err))
        }
    }
}
```

### 3. Extract Status for Reporting

```go
// Get tracked resources as variables
trackedVars := manager.GetTrackedResourcesAsVariables()

// Build complete variables map for expression evaluation
allVars := map[string]interface{}{
    "event": eventData,
    "env": envVars,
    "cluster": clusterData,
}

// Merge tracked resources
for k, v := range trackedVars {
    allVars[k] = v
}

// Evaluate expressions
status := evaluator.EvaluateBool("jobCompleted", allVars)
```

</details>

## GroupVersionKind (GVK) Utilities

### Production Usage (Config-Driven)

<details>
<summary>Click to expand: GVK Usage Examples</summary>

**Production Code (Config-Driven):**

```go
// ✅ CORRECT: Extract from config (adapter-config-template.yaml)
// Config YAML:
//   - kind: "Deployment"
//     apiVersion: "apps/v1"

gvk, err := GVKFromKindAndApiVersion(resourceConfig.Kind, resourceConfig.ApiVersion)
if err != nil {
    return fmt.Errorf("invalid GVK in config: %w", err)
}

// Use GVK with client operations
obj, err := client.GetResource(ctx, gvk, namespace, name)
```

**Testing (Test Helpers Only):**

```go
// ⚠️ TESTING ONLY - Do NOT use in production code
// Available in: internal/k8s-client/*_test.go and test/integration/k8s-client/*_test.go

// Core resources
gvk := CommonResourceKinds.Namespace
gvk := CommonResourceKinds.Pod
gvk := CommonResourceKinds.ConfigMap

// Apps resources
gvk := CommonResourceKinds.Deployment
gvk := CommonResourceKinds.Job
```

</details>

**Important:** `CommonResourceKinds` is in `test_helpers.go` and is for testing convenience only. Production code must use `GVKFromKindAndApiVersion()` with values from the adapter config (`kind` and `apiVersion` fields).

## Error Handling

All operations return descriptive errors:

```go
created, err := client.CreateResource(ctx, obj)
if err != nil {
    if errors.IsAlreadyExists(err) {
        // Resource already exists
    } else if errors.IsNotFound(err) {
        // Namespace or dependency not found
    } else {
        // Other error
    }
}
```

## Configuration

### Client Configuration

```go
config := ClientConfig{
    KubeConfigPath: "/path/to/kubeconfig", // or "" for in-cluster
    QPS: 100.0,  // Queries per second
    Burst: 200,  // Burst rate
}
```

### Authentication Methods

- **In-cluster / ServiceAccount** (KubeConfigPath = ""): 
  - Automatically uses pod's ServiceAccount token and cluster CA
  - Token is automatically mounted by Kubernetes
  - Requires RBAC permissions (ClusterRole/Role)
  - **Use this for production deployments**
  
- **Kubeconfig** (KubeConfigPath set): 
  - Uses specified kubeconfig file for authentication
  - Supports multiple clusters and contexts
  - **Use this for local development**

The client automatically detects which method to use based on configuration.

## Dependencies

Required packages:
- `k8s.io/client-go` - Kubernetes Go client
- `k8s.io/apimachinery` - Kubernetes API machinery
- `k8s.io/api` - Kubernetes API types
- `github.com/Masterminds/sprig/v3` - Template functions

## Testing

Unit tests should cover:
- Template rendering with various variables
- Resource creation and retrieval
- Discovery by name and selectors
- Status extraction
- Error handling

See `*_test.go` files for examples.

## Best Practices

1. **Use ResourceManager** for high-level operations
2. **Track resources** only when needed for status evaluation
3. **Refresh tracked resources** before evaluating status
4. **Use discovery configs** for idempotent resource lookup
5. **Set appropriate rate limits** to avoid overwhelming the API server
6. **Handle errors gracefully** - some resources may not exist yet
7. **Clear tracker** after processing each event to prevent memory leaks

## Memory Management

- Tracker clears resources after each event processing
- Use function-scoped variables (auto garbage collected)
- No persistent caching of resources (stateless design)
- Bounded concurrency prevents memory exhaustion

## Examples

See the `examples/` directory (if available) or the integration tests for complete usage examples.

