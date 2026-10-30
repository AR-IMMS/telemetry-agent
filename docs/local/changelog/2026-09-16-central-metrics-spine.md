# Changelog

All notable changes to this project are documented here.

## [Unreleased]

### Added

- Added a cross-platform `hostmetrics` baseline for CPU, memory, disk,
  filesystem, network, and paging telemetry.
- Added stable Agent identity attributes to host metrics.
- Added a Docker Compose observability stack containing an OTel Gateway,
  Prometheus, and Grafana, including Gateway Prometheus scrape configuration
  and Grafana Prometheus datasource provisioning.
- Added renderer contracts for Linux and Windows `hostmetrics` configurations,
  including a contract that prevents accidental TLS use with the plaintext
  homelab Gateway.

### Changed

- Configured OTLP/gRPC export to use plaintext transport for the trusted
  homelab proof of concept.
- Configured the co-located Windows proof to use Gateway ingress port `14317`
  while retaining the Agent's local OTLP port `4317`; a separate Ubuntu Gateway
  host uses LAN port `4317`.

### Fixed

- No changes.

### Deprecated

- No changes.

### Removed

- No changes.

### Security

- No changes.

### Breaking Changes

- No changes.
