# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-03-30

### Added

- `now()` custom CEL function returning current RFC3339 timestamp for time-based preconditions ([HYPERFLEET-763](https://issues.redhat.com/browse/HYPERFLEET-763), [#77](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/77))
  - Enables preconditions like "cluster has been in state X for at least N minutes" by exposing the current wall-clock time to CEL expressions.
- ServiceMonitor and Service resources in Helm chart for Prometheus Operator auto-discovery ([HYPERFLEET-586](https://issues.redhat.com/browse/HYPERFLEET-586), [#75](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/75))
  - Helm chart now provisions a `Service` (ports 8080/9090) and a `ServiceMonitor` so Prometheus Operator discovers and scrapes the adapter without manual configuration; skipped silently on clusters without the Operator CRD.
- Flat YAML configuration standard replacing Kubernetes resource envelope — **breaking change for adapters** ([HYPERFLEET-551](https://issues.redhat.com/browse/HYPERFLEET-551), [#67](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/67))
  - Replaces the `apiVersion/kind/metadata/spec` envelope with a plain snake_case YAML structure; all `AdapterConfig` and `AdapterTaskConfig` files must be migrated; a `/config` debug endpoint is added to inspect the live merged config.
- Create reliability documentation: runbook and metrics reference ([HYPERFLEET-585](https://issues.redhat.com/browse/HYPERFLEET-585), [#68](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/68))
  - Adds `docs/metrics.md` (Prometheus labels, PromQL queries, alert rules) and `docs/runbook.md` (failure modes, troubleshooting steps, and escalation paths for on-call engineers).

### Fixed

- Consolidate duplicate resource operation log entries ([HYPERFLEET-630](https://issues.redhat.com/browse/HYPERFLEET-630), [#84](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/84))
  - Removes 25 redundant per-method log lines from the k8s and Maestro clients, leaving a single authoritative INFO log per operation at the executor layer; also fixes a race where concurrent `Create` calls on the same resource returned an error instead of a no-op skip.
- Preserve failed `executionStatus` when a precondition error occurs ([HYPERFLEET-702](https://issues.redhat.com/browse/HYPERFLEET-702), [#83](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/83))
  - The executor now correctly marks execution as failed when a precondition evaluation error occurs, rather than leaving the status in an inconsistent state.
- Set failure status on nested discovery key collision ([HYPERFLEET-769](https://issues.redhat.com/browse/HYPERFLEET-769), [#83](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/83))
  - When two discovered resources resolve to the same context key, the adapter now sets a failure status instead of silently overwriting one result with the other.
- Fix time-based stability precondition inconsistency for thresholds above 300s ([HYPERFLEET-708](https://issues.redhat.com/browse/HYPERFLEET-708), [#81](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/81))
  - Changes the time comparison from `>=` to `>` so a precondition requiring "stable for 5 minutes" only passes after the full threshold has elapsed, not at the exact boundary.
- Use `0.0.0-dev` version for dev image builds to avoid version collisions ([HYPERFLEET-734](https://issues.redhat.com/browse/HYPERFLEET-734), [#74](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/74))
  - The `image-dev` Makefile target now stamps images with `0.0.0-dev` instead of the latest git tag, preventing personal dev builds from being mistaken for release artifacts.

### Changed

- Standardize Helm chart `appVersion` and `image.tag` handling; require explicit `image.tag` ([HYPERFLEET-794](https://issues.redhat.com/browse/HYPERFLEET-794), [#86](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/86))
  - `appVersion` is set to `0.0.0-dev` in source (CI stamps the real version at release); the deployment template no longer falls back to `appVersion` for `image.tag`, and a validation guard now fails the render if `image.tag` is not explicitly provided.
- Align Helm chart values to camelCase convention — **breaking change for Helm users** ([HYPERFLEET-786](https://issues.redhat.com/browse/HYPERFLEET-786), [#85](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/85))
  - All `values.yaml` keys are renamed to camelCase (e.g. `projectId`, `deadLetterTopic`, `hyperfleetApi.baseUrl`); chart version bumped to `2.0.0`; deprecated snake_case keys are rejected at render time.
- Remove unused `GIT_TAG` Makefile variable and `Tag` struct field ([HYPERFLEET-725](https://issues.redhat.com/browse/HYPERFLEET-725), [#82](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/82))
  - Removes the never-read `GIT_TAG` Make variable and the `Tag` field from `pkg/version/version.go`, simplifying the version startup log and `version` command output.
- Align golangci-lint configuration with architecture standard and enable revive linter ([HYPERFLEET-769](https://issues.redhat.com/browse/HYPERFLEET-769), [#80](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/80), [#83](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/83))
  - Removes project-specific linter exclusions, enables `revive` with the HyperFleet rule set, renames five internal packages from `snake_case` to `camelCase` (`config_loader` → `configloader`, `hyperfleet_api` → `hyperfleetapi`, etc.), and resolves ~185 violations across 87 files.
- Align adapter docs with operational documentation standard ([HYPERFLEET-751](https://issues.redhat.com/browse/HYPERFLEET-751), [#76](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/76))
  - Renames `observability.md` → `metrics.md` and `metrics.md` → `alerts.md` to reflect actual content, adds audience headers to all doc files, and consolidates cross-references; no code changes.
- Document required `hyperfleet.io/generation` annotation ([#79](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/79))
  - Adds the mandatory `hyperfleet.io/generation` annotation to all managed-resource examples in the authoring guide, clarifying why it must be present on every resource the adapter manages.

## [0.1.1] - 2026-03-10

### Fixed

- Skip version validation for 0.0.0-dev development builds ([HYPERFLEET-728](https://issues.redhat.com/browse/HYPERFLEET-728), [#71](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/71))
  - The startup version check now treats `0.0.0-dev` as a development marker and skips validation, so untagged local builds no longer crash when the binary version does not match the config's declared version.
- Copy CA certificates from builder to ubi9-micro runtime image ([HYPERFLEET-730](https://issues.redhat.com/browse/HYPERFLEET-730), [#72](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/72))
  - Copies the CA trust bundle from the builder stage into the minimal `ubi9-micro` runtime image, fixing `x509: certificate signed by unknown authority` failures when connecting to external TLS endpoints such as Google Pub/Sub.
- Standardize version handling to avoid go-toolset base image collision ([HYPERFLEET-723](https://issues.redhat.com/browse/HYPERFLEET-723), [#70](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/70))
  - Renames the Makefile version variable to `APP_VERSION` and pins it explicitly in the Dockerfile, preventing the UBI9 `go-toolset` base image's `ENV VERSION` from silently overwriting the adapter's version string.
- Rename generation variable to avoid conflicts ([HYPERFLEET-653](https://issues.redhat.com/browse/HYPERFLEET-653), [#52](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/52))
  - Unifies three inconsistent generation field name variants (`generation`, `generationSpec`, `generationValue`) across all config, template, and testdata YAML files to a single canonical name.
- Fix discovery byName example ([HYPERFLEET-660](https://issues.redhat.com/browse/HYPERFLEET-660), [#54](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/54))
  - Updates the example `discovery.byName` reference to use the correct ManifestWork name format (`mw-{{ .clusterId }}`), so the quickstart example works without modification.
- Prevent leaking Maestro SQL errors in API responses ([HYPERFLEET-661](https://issues.redhat.com/browse/HYPERFLEET-661), [#55](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/55))
  - Detects PostgreSQL foreign-key constraint errors from unregistered Maestro consumers and converts them to a clean `NotFound` error, preventing raw SQL details (table names, constraint names, SQLSTATE codes) from surfacing in gRPC responses.
- Fix Helm warning for duplicated volume projection ([HYPERFLEET-662](https://issues.redhat.com/browse/HYPERFLEET-662), [#56](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/56))
  - Removes a duplicate `adapter-config.yaml` key from the Helm projected volume definition, eliminating a Kubernetes warning on every `helm install` / `helm upgrade`.
- Change owned_reference to owner_references ([HYPERFLEET-663](https://issues.redhat.com/browse/HYPERFLEET-663), [#57](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/57))
  - Renames the misspelled `owned_reference` field to `owner_references` across executor types, business logic, and documentation to match the intended API contract.
- Fix RabbitMQ exchange type configuration ([HYPERFLEET-672](https://issues.redhat.com/browse/HYPERFLEET-672), [#58](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/58))
  - Adds a configurable `exchange_type` field (defaulting to `"topic"`) to the RabbitMQ Helm config so the adapter's declared exchange type matches the one provisioned by Sentinel, preventing a startup failure on type mismatch; also normalizes the `routing_key` field name.
- Fix examples using namespace in discovery ([HYPERFLEET-679](https://issues.redhat.com/browse/HYPERFLEET-679), [#59](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/59))
  - Removes the invalid `namespace: "*"` wildcard from example configs (it has no special meaning and would literally match a namespace named `*`), and clarifies namespace scoping behavior in the authoring guide.

### Added

- Prometheus metrics for event processing ([HYPERFLEET-450](https://issues.redhat.com/browse/HYPERFLEET-450), [#66](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/66))
  - Introduces a new `pkg/metrics` package with a nil-safe `Recorder` and three metrics: `hyperfleet_adapter_events_processed_total`, `hyperfleet_adapter_event_processing_duration_seconds`, and `hyperfleet_adapter_errors_total`; the executor is instrumented and PromQL examples are added to the observability docs.
- Integration test with TLS for Maestro client setup ([#60](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/60))
  - Adds integration tests covering TLS CA cert configuration and connection timeout settings for the Maestro client, including certificate generation helpers and test container setup; also introduces the `ca_file` HTTP CA cert config option.
- Enable PodDisruptionBudget by default in Helm chart ([HYPERFLEET-584](https://issues.redhat.com/browse/HYPERFLEET-584), [#64](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/64))
  - Enables `podDisruptionBudget` with `minAvailable: 1` by default so production Helm deployments are protected from voluntary disruptions (node drain, rolling upgrades) out of the box.
- Improved developer experience with better task execution info ([HYPERFLEET-653](https://issues.redhat.com/browse/HYPERFLEET-653), [#51](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/51))
  - Adds `toJson` and `dig` CEL helper functions for JSON serialization and nested-map traversal; enriches discovery context with `manifest`, `statusFeedback`, and `conditions`; promotes discovery failures to hard errors; adds label-selector filtering to dry-run mode.
- Add documentation for unknown value handling ([HYPERFLEET-596](https://issues.redhat.com/browse/HYPERFLEET-596), [#53](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/53))
  - Clarifies that reporting `Unknown` status does not update cluster state in the HyperFleet API (reconciliation continues unmodified), and restructures the examples README with a comparison table.

### Changed

- Standardize Dockerfile and Makefile for building images ([HYPERFLEET-509](https://issues.redhat.com/browse/HYPERFLEET-509), [#62](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/62))
  - Switches to UBI9 Go toolset builder and `ubi9-micro` runtime with a non-root user (65532), adds BuildKit cache mounts and version ldflags, and replaces the hand-crafted Makefile with a standardized one adding `fmt-check`, `vet`, `lint-check`, and other targets.
- Updated OWNERS file ([HYPERFLEET-702](https://issues.redhat.com/browse/HYPERFLEET-702), [#65](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/65))
  - Adds Phuongnhat Nguyen (`pnguyen44`) and removes Alex Vulaj from the approvers and reviewers list.
- Move broker metrics documentation to docs/observability.md ([HYPERFLEET-676](https://issues.redhat.com/browse/HYPERFLEET-676), [#63](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/63))
  - Extracts broker metric descriptions from `configuration.md` into a dedicated `docs/observability.md`, providing a single home for all Prometheus metric documentation.
- Updated hyperfleet-broker to v1.1.0 with Prometheus metrics ([HYPERFLEET-676](https://issues.redhat.com/browse/HYPERFLEET-676), [#63](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/63))
  - Bumps `hyperfleet-broker` to v1.1.0, which requires a `MetricsRecorder` argument to `NewSubscriber`; the adapter is wired up accordingly, automatically exposing broker metrics (`messages_consumed_total`, `messages_published_total`, `errors_total`, `message_duration_seconds`) on `/metrics`.
- Align broker metrics docs with configuration.md ([HYPERFLEET-676](https://issues.redhat.com/browse/HYPERFLEET-676), [#63](https://github.com/openshift-hyperfleet/hyperfleet-adapter/pull/63))
  - Updates cross-references in `configuration.md` to point to the new `observability.md` location and ensures metric names, labels, and descriptions are consistent between the two documents.

## [0.1.0] - 2026-02-19

### Added

- Initial release of HyperFleet Adapter Framework
- Configuration-driven framework for cluster provisioning tasks
  - Adapters declare their full behavior in YAML config files; no custom Go code is required for common provisioning patterns.
- CloudEvent-based event processing with broker support
  - Receives CloudEvents from HyperFleet Sentinel via a RabbitMQ broker; each event triggers a structured execution pipeline.
- Parameter extraction phase from environment, events, and resource status
  - First pipeline phase; resolves named parameters from env vars, the event payload, and live Kubernetes resource status fields for use in subsequent phases.
- Decision phase with CEL expression evaluation
  - Second phase; evaluates CEL preconditions to decide whether execution should proceed, wait (retry), or fail immediately.
- Resource phase with Kubernetes and Maestro client support
  - Third phase; applies, discovers, or deletes Kubernetes resources and Maestro ManifestWork objects as declared in config.
- Status reporting to HyperFleet API
  - After each execution, pushes a structured status payload (outcome, resource details) back to the HyperFleet API.
- Structured logging with context-aware fields (txid, opid, adapter_id, cluster_id)
  - Every log line carries request-scoped fields for end-to-end correlation across distributed traces and log aggregators.
- OpenTelemetry tracing support
  - Distributed traces are emitted for each event execution; configurable OTLP exporter integrates with Jaeger, Tempo, or any OTLP-compatible backend.
- Health and metrics endpoints
  - Exposes `/healthz` (liveness), `/readyz` (readiness), and `/metrics` (Prometheus) HTTP endpoints on configurable ports.
- Helm chart for Kubernetes deployment
  - Production-ready Helm chart with configurable replicas, resource requests/limits, secrets management, and optional PodDisruptionBudget.
- Dry-run mode for local testing without infrastructure
  - Runs the full execution pipeline against pre-recorded API responses, allowing developers to test adapter logic without a live cluster or broker.
- Integration tests with Testcontainers and envtest
  - Tests spin up real RabbitMQ containers and a real Kubernetes API server (via `envtest`) for end-to-end validation without a cluster.
- Bingo-based tool dependency management
  - Build tools (golangci-lint, helm, etc.) are pinned in `.bingo/` manifests and installed with `make setup`, ensuring reproducible CI builds.
- Comprehensive README with quickstart guide
  - Covers prerequisites, a minimal adapter quickstart, and links to all reference documentation.
- CONTRIBUTING guide for developers
  - Documents the development workflow, testing strategy, PR checklist, and code conventions.
- Configuration reference documentation
  - Full reference for all `AdapterConfig` and `AdapterTaskConfig` fields with types, defaults, and examples.
- Adapter authoring guide
  - Step-by-step guide for writing a new adapter, covering parameter extraction, CEL expressions, and resource management.
- Metrics and alerts documentation
  - Reference for all Prometheus metrics with labels and PromQL queries, plus recommended alert rules.
- Operational runbook
  - On-call runbook covering failure modes, diagnostic commands, and escalation paths.

[0.2.0]: https://github.com/openshift-hyperfleet/hyperfleet-adapter/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/openshift-hyperfleet/hyperfleet-adapter/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/openshift-hyperfleet/hyperfleet-adapter/compare/v0.1.0-rc.1...v0.1.0
