# Managed Dependency Enable and Configure

**Date:** 2026-09-30  
**Status:** Completed

## Added

- Generic enable lifecycle support for Agent-owned dependencies.
- Platform enable adapters wired through the dependency catalog.
- `agentctl dependency configure` interactive selector for managed dependency
  enabled state.
- `agentctl dependency pending` visibility for scheduled host teardown.
- Regression coverage for enable ownership, terminal cancellation, no-op
  configuration, generation acknowledgement, and teardown completion.

## Changed

- Disable and uninstall lifecycle actions now share the same generic managed
  lifecycle coordinator as enable.
- Dependencies can be re-enabled without reinstalling when their Agent-owned
  resources remain available.
- CLI lifecycle actions use distinct command actions and runtime teardown
  actions to keep command orchestration separate from persisted teardown state.

## Safety

- Enable is refused when the disabled dependency has no Agent-owned resources.
- `dependency configure` applies only changed desired states.
- Terminal cancellation does not mutate lifecycle state or host resources.
- Physical disable and uninstall continue only after the Collector confirms the
  activated configuration generation.
