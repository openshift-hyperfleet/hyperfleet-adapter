# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2025-01-15

### Fixed
- Skip version validation for 0.0.0-dev development builds ([HYPERFLEET-728](https://issues.redhat.com/browse/HYPERFLEET-728))
- Copy CA certificates from builder to ubi9-micro runtime image ([HYPERFLEET-730](https://issues.redhat.com/browse/HYPERFLEET-730))
- Standardize version handling to avoid go-toolset base image collision ([HYPERFLEET-723](https://issues.redhat.com/browse/HYPERFLEET-723))
- Rename generation variable to avoid conflicts ([HYPERFLEET-653](https://issues.redhat.com/browse/HYPERFLEET-653))
- Fix discovery byName example ([HYPERFLEET-660](https://issues.redhat.com/browse/HYPERFLEET-660))
- Prevent leaking Maestro SQL errors in API responses ([HYPERFLEET-661](https://issues.redhat.com/browse/HYPERFLEET-661))
- Fix Helm warning for duplicated volume projection ([HYPERFLEET-662](https://issues.redhat.com/browse/HYPERFLEET-662))
- Change owned_reference to owner_references ([HYPERFLEET-663](https://issues.redhat.com/browse/HYPERFLEET-663))
- Fix RabbitMQ exchange type configuration ([HYPERFLEET-672](https://issues.redhat.com/browse/HYPERFLEET-672))
- Fix examples using namespace in discovery ([HYPERFLEET-679](https://issues.redhat.com/browse/HYPERFLEET-679))

### Added
- Prometheus metrics for event processing ([HYPERFLEET-450](https://issues.redhat.com/browse/HYPERFLEET-450))
- Integration test with TLS for Maestro client setup
- Enable PodDisruptionBudget by default in Helm chart ([HYPERFLEET-584](https://issues.redhat.com/browse/HYPERFLEET-584))
- Update hyperfleet-broker to v1.1.0 with Prometheus metrics ([HYPERFLEET-676](https://issues.redhat.com/browse/HYPERFLEET-676))
- Improve developer experience with better task execution info ([HYPERFLEET-653](https://issues.redhat.com/browse/HYPERFLEET-653))

### Changed
- Standardize Dockerfile and Makefile for building images ([HYPERFLEET-509](https://issues.redhat.com/browse/HYPERFLEET-509))
- Update OWNERS file ([HYPERFLEET-702](https://issues.redhat.com/browse/HYPERFLEET-702))
- Move broker metrics documentation to docs/observability.md ([HYPERFLEET-676](https://issues.redhat.com/browse/HYPERFLEET-676))

### Documentation
- Add documentation for unknown value handling ([HYPERFLEET-596](https://issues.redhat.com/browse/HYPERFLEET-596))
- Align broker metrics docs with configuration.md

## [0.1.0] - 2024-12-20

### Added
- Initial release of HyperFleet Adapter Framework
- Configuration-driven framework for cluster provisioning tasks
- CloudEvent-based event processing with broker support
- Parameter extraction phase from environment, events, and resource status
- Decision phase with CEL expression evaluation
- Resource phase with Kubernetes and Maestro client support
- Status reporting to HyperFleet API
- Structured logging with context-aware fields (txid, opid, adapter_id, cluster_id)
- OpenTelemetry tracing support
- Health and metrics endpoints
- Helm chart for Kubernetes deployment
- Dry-run mode for local testing without infrastructure
- Integration tests with Testcontainers and envtest
- Bingo-based tool dependency management

### Documentation
- Comprehensive README with quickstart guide
- CONTRIBUTING guide for developers
- Configuration reference documentation
- Adapter authoring guide
- Metrics and alerts documentation
- Operational runbook

## [0.1.0-rc.1] - 2024-12-10

### Added
- Release candidate for initial release
- Core adapter framework implementation
- Basic CloudEvent processing pipeline
- Kubernetes and Maestro client integrations
- Configuration loading and validation
- Unit and integration test suites

[Unreleased]: https://github.com/openshift-hyperfleet/hyperfleet-adapter/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/openshift-hyperfleet/hyperfleet-adapter/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/openshift-hyperfleet/hyperfleet-adapter/compare/v0.1.0-rc.1...v0.1.0
[0.1.0-rc.1]: https://github.com/openshift-hyperfleet/hyperfleet-adapter/releases/tag/v0.1.0-rc.1
