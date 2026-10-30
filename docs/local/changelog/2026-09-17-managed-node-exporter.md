# 2026-09-17 — Managed Node Exporter dependency

Added Linux Node Exporter as an Agent-managed dependency.

* `agentctl dependency install node-exporter` installs the pinned, verified
  `v1.12.1` Linux AMD64 archive.
* The Agent owns `/opt/ar-imms/node-exporter` and
  `ar-imms-node-exporter.service`.
* Installation requires root, starts a loopback-only service at
  `127.0.0.1:9100`, and verifies `/metrics`.
* Existing healthy installations are reused; unhealthy existing state is never
  overwritten.

## Review hardening

### Changed

* Healthy services now detect drift in the Agent-owned systemd unit and update
  it without downloading or reinstalling the Node Exporter archive.
* A unit update atomically writes the unit, reloads systemd, restarts the
  service, and verifies `/metrics`.
* A failed `daemon-reload` or restart stops the update flow; health verification
  is not attempted after a systemd failure.

### Security

* The Node Exporter service runs with `DynamicUser=yes`,
  `NoNewPrivileges=yes`, and `ProtectHome=yes`.
* Fresh archive installations publish the final installation directory as
  `0755`, allowing the dynamic service user to traverse and execute the binary
  without granting write access.

## Out of scope

Archive upgrades, uninstall, process-level telemetry capability, and systemd
registration for the OTel Agent itself remain out of scope.

