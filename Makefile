# Makefile for hyperfleet-adapter

# Project metadata
PROJECT_NAME := hyperfleet-adapter
VERSION ?= 0.0.1
IMAGE_REGISTRY ?= quay.io/openshift-hyperfleet
IMAGE_TAG ?= latest

# Build metadata
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null || echo "")
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# LDFLAGS for build
LDFLAGS := -w -s
LDFLAGS += -X github.com/openshift-hyperfleet/hyperfleet-adapter/cmd/adapter.version=$(VERSION)
LDFLAGS += -X github.com/openshift-hyperfleet/hyperfleet-adapter/cmd/adapter.commit=$(GIT_COMMIT)
LDFLAGS += -X github.com/openshift-hyperfleet/hyperfleet-adapter/cmd/adapter.buildDate=$(BUILD_DATE)
ifneq ($(GIT_TAG),)
LDFLAGS += -X github.com/openshift-hyperfleet/hyperfleet-adapter/cmd/adapter.tag=$(GIT_TAG)
endif

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOIMPORTS := goimports

# Test parameters
TEST_TIMEOUT := 30m
RACE_FLAG := -race
COVERAGE_OUT := coverage.out
COVERAGE_HTML := coverage.html

# Directories
# Find all Go packages, excluding vendor and test directories
PKG_DIRS := $(shell $(GOCMD) list ./... 2>/dev/null | grep -v /vendor/ | grep -v /test/ || echo "./...")

.PHONY: help
help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: test
test: ## Run unit tests with race detection
	@echo "Running unit tests..."
	$(GOTEST) -v $(RACE_FLAG) -timeout $(TEST_TIMEOUT) $(PKG_DIRS)

.PHONY: test-coverage
test-coverage: ## Run unit tests with coverage report
	@echo "Running unit tests with coverage..."
	$(GOTEST) -v $(RACE_FLAG) -timeout $(TEST_TIMEOUT) -coverprofile=$(COVERAGE_OUT) -covermode=atomic $(PKG_DIRS)
	@echo "Coverage report generated: $(COVERAGE_OUT)"
	@echo "To view HTML coverage report, run: make test-coverage-html"

.PHONY: test-coverage-html
test-coverage-html: test-coverage ## Generate HTML coverage report
	@echo "Generating HTML coverage report..."
	$(GOCMD) tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@echo "HTML coverage report generated: $(COVERAGE_HTML)"

.PHONY: test-integration
test-integration: ## 🔧 Run integration tests (requires envtest binaries)
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "🔧 Running Integration Tests with envtest"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "ℹ️  Integration tests require Kubernetes API binaries (etcd, kube-apiserver)"
	@echo ""
	@if [ -z "$$KUBEBUILDER_ASSETS" ] && ! command -v setup-envtest > /dev/null; then \
		echo "❌ ERROR: envtest binaries not found"; \
		echo ""; \
		echo "To install:"; \
		echo "  1. go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest"; \
		echo "  2. setup-envtest use 1.31.x"; \
		echo "  3. export KUBEBUILDER_ASSETS=\$$(setup-envtest use -i -p path 1.31.x)"; \
		echo ""; \
		echo "For detailed setup instructions, see:"; \
		echo "  📖 test/integration/k8s-client/SETUP_INTEGRATION_TESTS.md"; \
		echo ""; \
		exit 1; \
	fi; \
	if [ -z "$$KUBEBUILDER_ASSETS" ] && command -v setup-envtest > /dev/null; then \
		echo "🔍 KUBEBUILDER_ASSETS not set, detecting automatically..."; \
		KUBEBUILDER_ASSETS=$$(setup-envtest use -i -p path 2>/dev/null); \
		if [ -z "$$KUBEBUILDER_ASSETS" ]; then \
			echo "❌ No envtest binaries found. Installing..."; \
			setup-envtest use 1.31.x; \
			KUBEBUILDER_ASSETS=$$(setup-envtest use -i -p path 1.31.x); \
		fi; \
		export KUBEBUILDER_ASSETS; \
		echo "✅ Using KUBEBUILDER_ASSETS=$$KUBEBUILDER_ASSETS"; \
		echo "🚀 Starting integration tests..."; \
		echo ""; \
		KUBEBUILDER_ASSETS=$$KUBEBUILDER_ASSETS $(GOTEST) -v -tags=integration ./test/integration/... -timeout $(TEST_TIMEOUT) || exit 1; \
	else \
		echo "✅ Using KUBEBUILDER_ASSETS=$$KUBEBUILDER_ASSETS"; \
		echo "🚀 Starting integration tests..."; \
		echo ""; \
		$(GOTEST) -v -tags=integration ./test/integration/... -timeout $(TEST_TIMEOUT) || exit 1; \
	fi; \
	echo ""; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	echo "✅ Integration tests passed!"; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

.PHONY: test-all
test-all: test test-integration ## ✅ Run ALL tests (unit + integration)
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "✅ All tests completed successfully!"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

.PHONY: lint
lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "Error: golangci-lint not found. Please install it:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

.PHONY: fmt
fmt: ## Format code with gofmt and goimports
	@echo "Formatting code..."
	@if command -v $(GOIMPORTS) > /dev/null; then \
		$(GOIMPORTS) -w .; \
	else \
		$(GOFMT) -w .; \
	fi

.PHONY: mod-tidy
mod-tidy: ## Tidy Go module dependencies
	@echo "Tidying Go modules..."
	$(GOMOD) tidy
	$(GOMOD) verify

.PHONY: build
build: ## Build binary
	@echo "Building $(PROJECT_NAME)..."
	@echo "Version: $(VERSION), Commit: $(GIT_COMMIT), BuildDate: $(BUILD_DATE)"
	@mkdir -p bin
	CGO_ENABLED=0 $(GOBUILD) -ldflags="$(LDFLAGS)" -o bin/$(PROJECT_NAME) ./cmd/adapter

.PHONY: clean
clean: ## Clean build artifacts and test coverage files
	@echo "Cleaning..."
	rm -rf bin/
	rm -f $(COVERAGE_OUT) $(COVERAGE_HTML)

.PHONY: docker-build
docker-build: ## Build docker image
	docker build -t $(PROJECT_NAME):$(VERSION) .

.PHONY: docker-push
docker-push: ## Push Docker image
	@echo "Pushing Docker image..."
	docker push $(IMAGE_REGISTRY)/$(PROJECT_NAME):$(IMAGE_TAG)
	@echo "Docker image pushed: $(IMAGE_REGISTRY)/$(PROJECT_NAME):$(IMAGE_TAG)"

.PHONY: verify
verify: lint test ## Run all verification checks (lint + test)

