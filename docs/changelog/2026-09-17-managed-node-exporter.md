
# 2026-09-17 — Managed Node Exporter dependency

Added Linux Node Exporter as an Agent-managed dependency.

- `agentctl dependency install node-exporter` installs the pinned, verified
  `v1.12.1` Linux AMD64 archive.
- The Agent owns `/opt/ar-imms/node-exporter` and
  `ar-imms-node-exporter.service`.
- Installation requires root, starts a loopback-only service at
  `127.0.0.1:9100`, and verifies `/metrics`.
- Existing healthy installations are reused; unhealthy existing state is never
  overwritten.
- Linux OTel config scrapes `prometheus/node_exporter` and forwards `node_*`
  metrics through the gateway to Prometheus and Grafana.
- The baseline Linux hostmetrics config disables the privileged process scraper.

Out of scope: upgrades, uninstall, process-level telemetry capability, and
systemd registration for the OTel Agent itself.
