# 2026-09-16 — Windows Exporter Managed Dependency

## Added

- `agentctl dependency install windows-exporter`.
- Administrator-only installation and reconciliation workflow.
- Agent-owned Windows Exporter configuration bound to `127.0.0.1:9182`.
- Version- and SHA-256-pinned Windows Exporter v0.31.8 MSI download.
- MSI staging, non-interactive installation, service inspection and health check.
- Safe reconciliation: reuse healthy service, install absent service, fail for an
  unhealthy existing service.
- Windows OTel configuration now scrapes Windows Exporter through
  `prometheus/windows_exporter`.

## Verified

The following telemetry path was verified on Windows:

```text
windows_exporter
→ OTel Agent
→ OTel Gateway
→ Prometheus
```

`windows_cpu_time_total` arrived in Prometheus with Agent-owned `host_id` and
`host_name` labels.

## Not included

LHM, Node Exporter, dependency upgrade/repair/uninstall, automatic elevation,
and self-installation of the OTel Agent as a Windows Service remain future work.
