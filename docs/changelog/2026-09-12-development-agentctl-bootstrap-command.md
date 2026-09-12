# Changelog

All notable changes to this project are documented here.

## [Unreleased]

### Added

- Added the development-only `agentctl bootstrap` command. It resolves the
  base, laptop-profile, and detected Linux or Windows Collector configuration
  layers; invokes the existing bootstrap flow; and reports the Collector
  version, binary path, final configuration path, and reuse status.
- Added offline command tests for configuration-layer selection, validation
  endpoint and timeout options, usage errors, bootstrap failures, and success
  output. The tests do not download an artifact or execute a Collector binary.

### Changed

- No changes.

### Fixed

- No changes.

### Deprecated

- No changes.

### Removed

- No changes.

### Security

- The validation endpoint is passed only to Collector validation through
  `OTEL_GATEWAY_ENDPOINT`; it is not added to rendered configuration.

### Breaking Changes

- No changes.
