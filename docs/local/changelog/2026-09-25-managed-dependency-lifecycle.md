# Managed Dependency Lifecycle

**Date:** 2026-09-25
**Status:** Completed

## Added

- `agentctl dependency status` for read-only lifecycle reporting.
- Interactive multi-select dependency installation in a terminal.
- Shared catalog-driven installers and inspectors for built-in integrations.
- Lifecycle output that reports availability, health, and configuration drift
  independently.

## Changed

- Dependency installation now reconciles known managed state instead of always
  treating an existing integration as a fresh install.
- Catalog integrations must provide an inspector as well as an installer.
- Windows Exporter can repair known Agent-owned configuration drift, including
  stale configuration that prevents the service from starting.

## Safety

- Inspection does not change the host.
- Reconciliation reuses healthy matching installations.
- Unsafe overwrite of unhealthy or unowned existing state is refused.
