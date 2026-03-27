# Claude Code Context for HyperFleet Adapter

This file provides AI agent context for working with the HyperFleet Adapter codebase.

## Quick Reference

**Language:** Go 1.25.0+
**Build System:** Make
**Test Framework:** Go testing + Testcontainers
**Linter:** golangci-lint (managed via bingo)

## Essential Commands

### Build and Verify
```bash
make build           # Build binary → bin/hyperfleet-adapter
make fmt             # Format code with goimports
make lint            # Run golangci-lint
make tidy            # Tidy dependencies
make verify          # Run fmt-check and vet
```

### Testing
```bash
make test                    # Unit tests only (fast)
make test-integration        # Integration tests with envtest (unprivileged, CI-safe)
make test-integration-k3s    # Integration tests with K3s (faster, needs privileges)
make test-all                # All tests (unit + integration)
make test-coverage           # Generate coverage report
```

### Container Images
```bash
make image                   # Build image
make image-push              # Build and push to quay.io/openshift-hyperfleet/hyperfleet-adapter
QUAY_USER=myuser make image-dev  # Build and push to personal Quay
```

## Validation Checklist

Before committing code, run these in order:
1. `make fmt` - Format code
2. `make lint` - Check linting
3. `make test` - Unit tests
4. `make test-integration` - Integration tests (or `make test-all`)
5. `make build` - Ensure binary builds

## Project Structure

```
pkg/             # Exported packages (can be imported by other projects)
├── constants/   # Shared constants (annotations, labels)
├── errors/      # Error handling utilities with codes
├── health/      # Health and metrics HTTP servers
├── logger/      # Structured logging with context
├── otel/        # OpenTelemetry tracing
├── utils/       # General utilities
└── version/     # Version info

internal/        # Internal packages (not importable)
├── config_loader/    # YAML config loading + validation
├── criteria/         # Precondition and CEL evaluation
├── executor/         # Event execution pipeline (phases)
├── hyperfleet_api/   # HyperFleet API client
├── k8s_client/       # Kubernetes client wrapper
├── maestro_client/   # Maestro/OCM ManifestWork client
├── manifest/         # Manifest generation and rendering
└── transport_client/ # Unified apply interface

cmd/adapter/     # Main entry point
test/integration/ # Integration tests
charts/          # Helm chart
configs/         # Config templates and examples
scripts/         # Build scripts
```

## Code Conventions

### Logging
Always use structured logging with context:
```go
logger.InfoContext(ctx, "message", "key1", val1, "key2", val2)
logger.ErrorContext(ctx, "error occurred", "error", err, "resource", name)
```

Never use `fmt.Printf` or `log.Println` - use the logger package.

### Error Handling
Use structured errors from `pkg/errors`:
```go
return errors.NewNotFoundError("CLUSTER_NOT_FOUND", "cluster not found",
    errors.WithResourceID(clusterID))
```

Always propagate errors up with context, don't silently ignore them.

### Context Propagation
All long-running operations must accept `context.Context` as first parameter:
```go
func ProcessEvent(ctx context.Context, event cloudevents.Event) error
```

Use `logger.WithContext(ctx)` to extract logger with operation ID.

### CEL Expressions
CEL expressions in config must use exact field names from the CEL environment:
- `params.*` - extracted parameters
- `event.*` - CloudEvent fields
- `resources.*` - discovered resources (post-phase)
- Custom functions: `now()` for time-based conditions

### Resource Names
Configuration uses **snake_case** (Viper convention):
- `adapter.name`, `clients.hyperfleet_api.base_url`

Code uses **camelCase** (Go convention):
- `AdapterName`, `HyperFleetAPIBaseURL`

## Boundaries - Do NOT Do This

### Generated Files
- **Do not modify** files in `.bingo/` - these are managed by bingo
- **Do not edit** `go.sum` manually - use `make mod-tidy`

### Testing
- **Do not skip integration tests** in PRs - they run in CI and catch real issues
- **Do not mock Kubernetes clients** in integration tests - use envtest/K3s
- **Do not use time.Sleep** in tests - use context timeouts or test utilities

### Configuration
- **Do not add hardcoded values** - use configuration or environment variables
- **Do not add CLI flags** without also supporting env vars (Viper convention)
- **Do not change config field names** without migration path (breaking change)

### Dependencies
- **Do not add dependencies** without license check (Apache 2.0 compatible only)
- **Do not update hyperfleet-broker** without verifying metric compatibility
- **Do not vendor dependencies** - this project uses Go modules

### Git
- **Do not force push** to main or release branches
- **Do not amend published commits** - create new commits
- **Do not commit** `.env` files, credentials, or sensitive data

### API Changes
- **Do not break backward compatibility** in config schema without version bump
- **Do not change CloudEvent types** without coordinating with HyperFleet API team
- **Do not modify status payload schema** without API spec update

## Testing Patterns

### Table-Driven Tests
Use subtests for multiple scenarios:
```go
func TestFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid", "input", "output", false},
        {"invalid", "", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Function(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("unexpected error: %v", err)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Integration Test Setup
Use testcontainers for real dependencies:
```go
func TestIntegration(t *testing.T) {
    ctx := context.Background()
    env := testutil.SetupEnvTest(t, ctx) // Sets up K8s API
    defer env.Teardown()
    // Test with real K8s client
}
```

## Common Tasks

### Adding a New Parameter Extractor
1. Add extractor to `internal/config_loader/param_extractors.go`
2. Add tests in same package
3. Document in `docs/configuration.md`
4. Add example in `configs/adapter-task-config-template.yaml`

### Adding a New Precondition Type
1. Add evaluator to `internal/criteria/precondition_evaluators.go`
2. Add tests with mock clients
3. Update schema in `internal/config_loader/schema.go`
4. Document in adapter authoring guide

### Adding Metrics
1. Define metric in `pkg/health/metrics.go`
2. Instrument code with metric calls
3. Add Prometheus query to `docs/metrics.md`
4. Add recommended alert to `docs/alerts.md`

## Release Process

Version follows semver (MAJOR.MINOR.PATCH):
- Update version in `Makefile` (`VERSION` variable)
- Update `CHANGELOG.md` with release notes
- Tag: `git tag -a v0.2.0 -m "Release v0.2.0"`
- Push: `git push origin v0.2.0`
- CI builds and pushes image with version tag

## Links

- [Architecture Docs](https://github.com/openshift-hyperfleet/architecture)
- [HyperFleet API Spec](https://github.com/openshift-hyperfleet/hyperfleet-api-spec)
- [Broker Library](https://github.com/openshift-hyperfleet/hyperfleet-broker)
