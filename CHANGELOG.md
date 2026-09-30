# Changelog

All notable changes to this project are documented here.

## [Unreleased]

## [0.1.2] - 2026-09-28

### Added

- Added dependency lifecycle commands for `disable`, `uninstall`, and
  `pending`.
- Added safe teardown adapters for Windows Exporter and Libre Hardware Monitor.
- Added persisted ownership checks before physical dependency cleanup.
- Added lifecycle documentation and operator runbook diagrams.

### Changed

- Collector configuration now activates dependency changes before the runtime
  restarts for the new generation.
- Dependency status now distinguishes dependencies that are `not installed`
  from dependencies tracked as disabled.
- Dependency status accepts `--state-path` for explicit lifecycle-state
  inspection.

### Fixed

- Fixed Windows Exporter MSI uninstall when its installer recreates a missing
  installation directory during removal.
- Fixed CLI tests to use temporary lifecycle state instead of the production
  state path.

### Security

- Physical teardown is limited to resources recorded as Agent-owned during a
  fresh installation.
