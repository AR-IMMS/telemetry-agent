## [0.1.4a-next] - Unreleased

This is the release-candidate changelog for the `chore/v0.1.4a-next`
integration branch. It describes the target contents; this version is not
published yet.

### Highlights

- Install, upgrade, recover, and uninstall the Agent and its managed runtime on
  Windows and Linux while preserving installation identity and owned resources.
- Enroll Agents with protected credentials and retain local recovery behavior
  when control-plane connectivity is unavailable.
- Use the pinned OpenTelemetry Collector and OpAMP Supervisor for supported
  native runtime flows, with validation, readiness checks, last-known-good
  recovery, and an explicit legacy ownership path.
- Monitor hosts and supported workloads with OpenTelemetry-native collection,
  including Docker-discovered Redis and PostgreSQL profiles and generic
  Prometheus endpoints where configured and reachable.
- Retain Agent-managed Windows Exporter and Libre Hardware Monitor support, and
  optional Linux Node Exporter integration, alongside Collector-managed
  collection.
- Inspect inventory, health evidence, and diagnostics; execute authorized
  infrastructure operations through the existing Core-authorized Operations
  channel. Mutating container operations remain disabled by default.
- Preserve existing `agentctl` commands and behavior while organizing newer
  commands around the shared application use cases.

### Developer notes

- Keep Fleet enrollment, desired Collector configuration, and publication
  contracts separate from Core business authorization and local Agent
  orchestration. The Mock Fleet is test-only and provisional; production
  Fleet/NestJS interoperability is not verified by Mock Fleet tests.
- Existing SQLite state and migration history remain supported. Agent-managed
  exporters and OS integration remain distinct from Collector lifecycle
  ownership.
- Verification covers Windows and Ubuntu CI, including live Ubuntu Docker,
  JetStream, Collector/OTLP, Supervisor fallback, and Mock Fleet E2Es. Windows
  Docker-dependent hosted E2Es are skipped; see the PR-09.5 verification report
  for exact-head evidence and limitations.

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
