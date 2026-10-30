# 2026-10-01 — Windows Agent lifecycle and clean uninstall

## Delivered

- Verified Windows clean install through local Gateway, Prometheus, and Grafana.
- Managed Windows Exporter and Libre Hardware Monitor through install, disable, enable, and uninstall.
- Added safe dependency teardown after the replacement Collector generation becomes ready.
- Stabilized Windows Collector restart when graceful console shutdown is unavailable in a Windows service.
- Aligned the default Agent state path with the packaged installation layout.
- Verified clean Agent uninstall from an external binary.

## Verified behavior

- `desiredGeneration`, `activatedGeneration`, and `appliedGeneration` converge after each lifecycle action.
- Disabling a dependency keeps the Agent running and removes future telemetry samples.
- Uninstalling a dependency removes only Agent-owned resources.
- Clean Agent uninstall removes the Agent service, binary, state directory, and managed dependency resources.

## Known limitation

Agent health is available through `GET /v1/status` on port `13134`, but it is not yet exported as a dedicated Prometheus heartbeat metric. Grafana cannot currently represent Agent health independently from managed dependency telemetry.
