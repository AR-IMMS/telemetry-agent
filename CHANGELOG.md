## [0.1.2] - 2026-10-01

### Added

- Added dependency lifecycle commands for `disable`, `uninstall`, and
  `pending`.
- Added safe teardown adapters for Windows Exporter and Libre Hardware Monitor.
- Added persisted ownership checks before physical dependency cleanup.
- Added lifecycle documentation and operator runbook diagrams.
- Added Windows service regression coverage for Agent runtime lifetime.

### Changed

- Collector configuration now activates dependency changes before the runtime
  restarts for the new generation.
- Dependency status now distinguishes dependencies that are `not installed`
  from dependencies tracked as disabled.
- Dependency status accepts `--state-path` for explicit lifecycle-state
  inspection.
- Canonical Agent state paths now match packaged installation paths on Windows
  and Linux.
- Windows service lifetime is controlled by SCM requests instead of the
  command-process context.
- Documented the local metrics path from managed dependencies through Gateway,
  Prometheus, and Grafana.

### Fixed

- Fixed Windows Exporter MSI uninstall when its installer recreates a missing
  installation directory during removal.
- Fixed CLI tests to use temporary lifecycle state instead of the production
  state path.
- Fixed Collector generation restart when a Windows service cannot send a
  console shutdown signal to its child process.
- Fixed Linux path expectations when the test suite runs on Windows.

### Security

- Physical teardown is limited to resources recorded as Agent-owned during a
  fresh installation.
