# Contributing to HyperFleet Adapter

Thank you for your interest in contributing to HyperFleet Adapter! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Code Style](#code-style)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)
- [Pull Request Process](#pull-request-process)

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/). By participating, you are expected to uphold this code.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:

   ```bash
   git clone https://github.com/YOUR_USERNAME/hyperfleet-adapter.git
   cd hyperfleet-adapter
   ```

3. **Add the upstream remote**:

   ```bash
   git remote add upstream https://github.com/openshift-hyperfleet/hyperfleet-adapter.git
   ```

4. **Create a branch** for your changes:

   ```bash
   git checkout -b your-feature-branch
   ```

## Development Setup

### Prerequisites

- Go 1.26 or later
- Docker (for building Docker images)
- `golangci-lint` (for linting)
- `make` (for running Makefile targets)
- `pre-commit` (for local git hooks)

### Installing Dependencies

1. **Install Go dependencies**:

   ```bash
   make tidy
   ```

2. **Install golangci-lint** (if not already installed):

   ```bash
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   ```

3. **Install git hooks**:

   ```bash
   make install-hooks
   ```

   This installs pre-commit hooks configured in `.pre-commit-config.yaml` for commit message and code quality validation on every commit.

### Building the Project

Production builds and tests select Go's native FIPS 140-3 module (`GOFIPS140=v1.0.0`). The `godebug fips140=on` directive in `go.mod` enables FIPS mode by default, including on the Red Hat Go toolset (which does not enable it merely by selecting the module). `GOEXPERIMENT=noboringcrypto` keeps builds compatible with toolchains where the legacy experiment was saved with `go env -w`. The Dockerfile checks both the selected module and `DefaultGODEBUG=fips140=on`.

Red Hat Go toolsets use an OpenSSL backend by default on Linux with cgo.
That backend aborts on FIPS-enabled hosts when `fips140` GODEBUG is set.
Make defaults to `no_openssl` for builds and tests on every Go toolchain;
the tag is harmless on upstream Go, so no vendor detection is needed.
Tests receive tags separately, without production-only `-trimpath`.
Make exports CGO and FIPS settings once for all recipes and tools.
Overrides replace the defaults; callers own the complete replacement,
including `no_openssl` in `GOFLAGS` and `GO_TEST_TAGS` when changing tags.
Empty overrides are not normalized.
Integration scripts preserve these tags alongside `integration`.
The Dockerfile checks the exact `no_openssl` token.
Make enforces `GOWORK=off` to honor `go.mod`'s FIPS default.
Prefer Make; direct Go commands do not inherit its defaults. Configure the same
settings and include the native backend tag explicitly:

```bash
export CGO_ENABLED=1 GOEXPERIMENT=noboringcrypto GOFIPS140=v1.0.0 GOWORK=off
# Compile check only; make build produces the service executable.
go build -trimpath -tags=no_openssl ./...
go test -tags=no_openssl ./...
```

Do not set `fips140ems=0` through `GODEBUG`, `godebug` in `go.mod`/`go.work`,
or `//go:debug` in the main package: it disables TLS 1.2 EMS enforcement.
The Dockerfile rejects this compiled override even alongside `fips140=on`.

FIPS production deployments must not disable that compiled default via `GODEBUG=fips140=off` or `GODEBUG=fips140=` (including within comma-separated values), a `godebug` directive in `go.work`, or a `//go:debug` directive in the main package. The Dockerfile checks the compiled default, not runtime overrides; an entirely empty or unset `GODEBUG` preserves the compiled default.

```bash
# Build the binary
make build

# The binary will be created at: bin/hyperfleet-adapter
```

### Running Tests

```bash
# Run unit tests
make test

# Run integration tests
make test-integration

# Run all tests
make test-all

# Run tests with coverage
make test-coverage
```

### Linting

```bash
# Run linter
make lint

# Format code
make fmt
```

## Making Changes

### Branch Naming

Use descriptive branch names that indicate the type of change:

- `feature/your-feature-name` - For new features
- `fix/your-fix-name` - For bug fixes
- `docs/your-doc-update` - For documentation changes
- `refactor/your-refactor-name` - For code refactoring

### Commit Messages

Follow the [commit standard](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/commit-standard.md) in our architecture repository.

## Code Style

### Go Code Style

- Follow the [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` or `goimports` to format code
- Run `make fmt` before committing

### General Guidelines

1. **Keep functions small and focused** - Each function should do one thing well
2. **Use meaningful variable names** - Avoid abbreviations unless they're widely understood
3. **Add comments** for exported functions, types, and packages
4. **Handle errors explicitly** - Don't ignore errors
5. **Use context.Context** for cancellation and timeouts in long-running operations

### Package Organization

- `cmd/` - Main applications for this project
- `pkg/` - Library code that can be imported by other projects
- `test/` - Integration tests and test utilities
- `data/` - Configuration templates and data files
- `charts/` - Helm chart templates

## Testing

### Unit Tests

- Write unit tests for all new functionality
- Use table-driven tests when testing multiple scenarios
- Run tests before submitting: `make test`

### Integration Tests

- Write integration tests for components that interact with external systems
- Use testcontainers for testing with real dependencies (e.g., databases, message brokers)
- Run integration tests: `make test-integration`

### Test Guidelines

1. **Test names should be descriptive**: `TestFunctionName_Scenario_ExpectedBehavior`
2. **Use subtests** for multiple test cases:

   ```go
   func TestFunction(t *testing.T) {
       tests := []struct {
           name string
           // ... test cases
       }{
           // ... test data
       }

       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               // test implementation
           })
       }
   }
   ```

3. **Clean up resources** in tests (use `defer` or `t.Cleanup()`)

### Chart Documentation

The Helm chart README is auto-generated by [helm-docs](https://github.com/norwoodj/helm-docs) from annotations in `charts/values.yaml`. **Do not edit `charts/README.md` by hand** — your changes will be overwritten.

```bash
# Regenerate the chart README after modifying values.yaml
make helm-docs

# Verify the README is up to date (CI runs this automatically)
make verify-helm-docs
```

When adding or modifying values in `charts/values.yaml`, annotate each key with a `# -- ` comment directly above it:

```yaml
# -- Number of adapter replicas
replicaCount: 1

image:
  # -- Container image registry
  registry: quay.io
```

The `verify-helm-docs` check runs as part of `make test-helm` and will fail the PR if the README is out of date.

## Submitting Changes

### Before Submitting

1. **Update your branch** with the latest changes from upstream:

   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run all checks**:

   ```bash
   make verify  # Runs fmt-check and vet
   ```

3. **Ensure all tests pass**:

   ```bash
   make test-all
   ```

4. **Check for linting errors**:

   ```bash
   make lint
   ```

5. **Format your code**:

   ```bash
   make fmt
   ```

### Pull Request Process

1. **Push your changes** to your fork:

   ```bash
   git push origin your-feature-branch
   ```

2. **Create a Pull Request** on GitHub:
   - Use a clear and descriptive title
   - Reference any related issues in the description
   - Describe what changes you made and why
   - Include screenshots or examples if applicable

3. **PR Checklist**:
   - [ ] Code follows the project's style guidelines
   - [ ] Self-review completed
   - [ ] Comments added for complex code
   - [ ] Documentation updated (if needed)
   - [ ] Tests added/updated
   - [ ] All tests pass (`make test-all`)
   - [ ] Linting passes (`make lint`)
   - [ ] No merge conflicts with main branch

### PR Description Template

```markdown
## Description
Brief description of the changes

## Related Issues
Fixes #123
Related to #456

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
Describe the tests you ran and their results

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Tests added/updated
- [ ] Documentation updated
```

## Code Review

- All PRs require at least one approval before merging
- Address review comments promptly
- Be respectful and constructive in reviews
- Ask questions if something is unclear

## Questions?

If you have questions or need help:

1. Check existing issues and pull requests
2. Open a new issue with the `question` label
3. Reach out to maintainers
