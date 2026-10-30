# Changelog

All notable changes to this project are documented here.

## [Unreleased]

### Added

- Added Libre Hardware Monitor as an Agent-managed Windows dependency.
- Added `agentctl dependency install libre-hardware-monitor`, which installs
  the pinned, checksum-verified Libre Hardware Monitor `v0.9.6` release.
- Added Agent ownership of the LHM installation at
  `C:\Program Files\AR-IMMS\LibreHardwareMonitor`.
- Added LHM web configuration, a `LocalSystem` startup task, and verification
  of the local `/metrics` endpoint on TCP `9190`.
- Added Windows OTel scraping of LHM through
  `prometheus/libre_hardware_monitor`.

### Changed

- Healthy installations with managed task, configuration, or firewall drift are
  repaired without downloading the archive again.
- Archive upgrades, uninstall, sensor-selection policy, endpoint TLS or
  authentication, and process-level telemetry remain out of scope.

### Fixed

- No changes.

### Deprecated

- No changes.

### Removed

- No changes.

### Security

- Installation requires an Administrator terminal.
- LHM uses an upstream wildcard HTTP binding; the Agent creates an enabled
  Windows Firewall inbound-block rule for TCP `9190`.
- Only the local OTel Agent is intended to scrape `127.0.0.1:9190`.
- Existing unhealthy installations are never overwritten automatically.

### Breaking Changes

- No changes.
