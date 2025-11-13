# K8s Client Integration Tests

Integration tests for the k8s-client package using `envtest` (local Kubernetes API server).

## Files Structure

```
test/integration/k8s-client/
├── README.md                          # This file
├── helper.go                          # Test infrastructure (envtest setup)
├── client_integration_test.go         # Client CRUD tests (7 tests)
├── manager_integration_test.go        # Manager operation tests (5 tests)
└── tracker_integration_test.go        # Tracker/discovery tests (5 tests)
```

**Total: 17 integration tests**

## How to Run

### Prerequisites

```bash
# Install envtest tool
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Download Kubernetes binaries (etcd, kube-apiserver)
setup-envtest use 1.31.x

# Set environment variable
export KUBEBUILDER_ASSETS=$(setup-envtest use -i -p path 1.31.x)
```

### Run Integration Tests

```bash
# Via Makefile (recommended - handles KUBEBUILDER_ASSETS automatically)
make test-integration

# All integration tests (manual)
KUBEBUILDER_ASSETS=$(setup-envtest use -i -p path 1.31.x) \
  go test -v -tags=integration ./test/integration/k8s-client

# Specific test
KUBEBUILDER_ASSETS=$(setup-envtest use -i -p path 1.31.x) \
  go test -v -tags=integration ./test/integration/k8s-client -run TestIntegration_CreateResource

# With coverage
make test-coverage
```

### Expected Output

```
=== RUN   TestIntegration_NewClient
=== RUN   TestIntegration_NewClient/client_is_properly_initialized
--- PASS: TestIntegration_NewClient (0.02s)
    --- PASS: TestIntegration_NewClient/client_is_properly_initialized (0.00s)
=== RUN   TestIntegration_CreateResource
=== RUN   TestIntegration_CreateResource/create_namespace
=== RUN   TestIntegration_CreateResource/create_configmap
--- PASS: TestIntegration_CreateResource (0.05s)
    --- PASS: TestIntegration_CreateResource/create_namespace (0.02s)
    --- PASS: TestIntegration_CreateResource/create_configmap (0.03s)
...
PASS
ok      github.com/openshift-hyperfleet/hyperfleet-adapter/test/integration/k8s-client    3.456s
```


## What's Tested

### Client Tests (`client_integration_test.go`)
- ✅ Client initialization
- ✅ Create resource (Namespace, ConfigMap)
- ✅ Get resource
- ✅ List resources with label selectors
- ✅ Update resource
- ✅ Delete resource (including namespace termination handling)
- ✅ Patch resource

### Manager Tests (`manager_integration_test.go`)
- ✅ Create resource from template
- ✅ Create or update resource (idempotency)
- ✅ Resource exists check
- ✅ Discover and track resources
- ✅ Refresh tracked resources

### Tracker Tests (`tracker_integration_test.go`)
- ✅ Discover and track by name
- ✅ Discover and track by label selectors
- ✅ Refresh tracked resource after modification
- ✅ Refresh all tracked resources
- ✅ Extract status and fields

## Architecture Notes

**Separation of Concerns:**
- **Manager** handles template rendering and orchestration
- **Tracker** validates inputs and stores resources (no rendering)
- **Client** performs Kubernetes API operations

See `internal/k8s-client/SEPARATION_OF_CONCERNS.md` for details.

## Troubleshooting

### Tests Hang or Timeout

```bash
# Check envtest is installed
setup-envtest list

# Verify KUBEBUILDER_ASSETS is set
echo $KUBEBUILDER_ASSETS

# If empty, set it:
export KUBEBUILDER_ASSETS=$(setup-envtest use -i -p path 1.31.x)
```

### "fork/exec .../etcd: no such file or directory"

```bash
# KUBEBUILDER_ASSETS not set or incorrect
export KUBEBUILDER_ASSETS=$(setup-envtest use -i -p path 1.31.x)

# Verify the path contains binaries
ls -la $KUBEBUILDER_ASSETS
# Should show: etcd, kube-apiserver, kubectl
```

### Network/Proxy Issues Downloading envtest

If you have network/proxy issues downloading envtest binaries, configure your proxy settings:

```bash
# Set proxy environment variables
export HTTP_PROXY=http://proxy.example.com:8080
export HTTPS_PROXY=http://proxy.example.com:8080
export NO_PROXY=localhost,127.0.0.1

# Then download envtest
setup-envtest use 1.31.x
```

### "API Server Failed to Start"

```bash
# Clean up any stale processes
pkill -f "etcd|kube-apiserver"

# Re-download envtest binaries
setup-envtest cleanup
setup-envtest use 1.31.x

# Verify they work
make test-integration
```

### Tests Pass Locally but Fail in CI

Ensure your CI environment has `setup-envtest` installed and `KUBEBUILDER_ASSETS` set:

```bash
# In your CI/CD pipeline (e.g., GitHub Actions, GitLab CI)
- name: Setup envtest
  run: |
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    export KUBEBUILDER_ASSETS=$(setup-envtest use -i -p path 1.31.x)
    
- name: Run integration tests
  run: make test-integration
```


