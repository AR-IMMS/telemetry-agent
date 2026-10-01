# Agent Clean-Uninstall E2E Runbook

## Purpose

Verify that the Agent and its managed Node Exporter dependency can be installed, disabled, uninstalled, and removed without deleting user-owned source configuration.

This runbook was validated on Linux with the E2E binary at `.e2e/bin/agentctl`.

## Scope

This procedure verifies:

- Collector bootstrap and persisted state
- Node Exporter install, disable, and uninstall
- Node Exporter systemd unit, binary directory, and port cleanup
- Agent systemd unit, installation directory, and state directory cleanup
- Preservation of the repository-owned `configs/` directory

## Preconditions

- Run commands from the telemetry-agent repository root.
- Build the external E2E binary beforehand at `.e2e/bin/agentctl`.
- Use `sudo` for lifecycle commands.
- Use the explicit state path below for every lifecycle command.

```text
/var/lib/ar-imms/telemetry-agent/state.json
```

> The explicit state path is required until all `agentctl` commands share one canonical default. See [Agentctl Command and State Consistency Gaps](../architecture/agentctl-command-state-consistency.md).

## 1. Install Agent

```bash
sudo ./.e2e/bin/agentctl install --config-root "$PWD/configs"
```

Expected output includes:

```text
Agent installation complete: ar-imms-telemetry-agent
Agent state path: /var/lib/ar-imms/telemetry-agent/state.json
```

## 2. Bootstrap Collector

```bash
sudo /opt/ar-imms/telemetry-agent/agentctl bootstrap \
  --config-root "$PWD/configs" \
  --install-dir /opt/ar-imms/telemetry-agent/collector \
  --config-path /var/lib/ar-imms/telemetry-agent/collector.yaml \
  --state-path /var/lib/ar-imms/telemetry-agent/state.json
```

Expected output includes the Collector version, binary path, final configuration path, and whether the Collector installation was reused.

## 3. Install Node Exporter

```bash
sudo /opt/ar-imms/telemetry-agent/agentctl dependency install node-exporter \
  --state-path /var/lib/ar-imms/telemetry-agent/state.json
```

Verify that the endpoint is reachable:

```bash
curl -fsS http://127.0.0.1:9100/metrics >/dev/null && echo "node-exporter reachable"
```

## 4. Disable Node Exporter

```bash
sudo /opt/ar-imms/telemetry-agent/agentctl dependency disable node-exporter \
  --state-path /var/lib/ar-imms/telemetry-agent/state.json
```

Verify that port `9100` is closed:

```bash
curl http://127.0.0.1:9100/metrics
```

Expected result:

```text
curl: (7) Failed to connect to 127.0.0.1 port 9100
```

## 5. Uninstall Node Exporter

```bash
sudo /opt/ar-imms/telemetry-agent/agentctl dependency uninstall node-exporter \
  --state-path /var/lib/ar-imms/telemetry-agent/state.json
```

Verify dependency cleanup:

```bash
sudo sed -n '1,180p' /var/lib/ar-imms/telemetry-agent/state.json
sudo systemctl status ar-imms-node-exporter.service --no-pager
sudo ls -ld /opt/ar-imms/node-exporter
curl http://127.0.0.1:9100/metrics
```

Expected results:

- `dependencies` is `{}`.
- `desiredGeneration`, `activatedGeneration`, and `appliedGeneration` have the same value.
- `ar-imms-node-exporter.service` cannot be found.
- `/opt/ar-imms/node-exporter` does not exist.
- Port `9100` is closed.

## 6. Uninstall Agent

Agent uninstall must be run from an external binary. Do not run it from `/opt/ar-imms/telemetry-agent/agentctl`, because uninstall removes that installation.

```bash
sudo ./.e2e/bin/agentctl uninstall \
  --state-path /var/lib/ar-imms/telemetry-agent/state.json
```

Verify Agent cleanup:

```bash
sudo systemctl status ar-imms-telemetry-agent.service --no-pager
sudo ls -ld /opt/ar-imms/telemetry-agent
sudo ls -ld /var/lib/ar-imms/telemetry-agent
ls configs
```

Expected results:

- `ar-imms-telemetry-agent.service` cannot be found.
- `/opt/ar-imms/telemetry-agent` does not exist.
- `/var/lib/ar-imms/telemetry-agent` does not exist.
- The repository-owned `configs/` directory remains intact.

## Legacy Test Residue

If a previous interrupted test leaves an inactive `ar-imms-node-exporter.service` or `/opt/ar-imms/node-exporter` behind, inspect it before cleanup:

```bash
sudo systemctl status ar-imms-node-exporter.service --no-pager
sudo systemctl cat ar-imms-node-exporter.service
sudo ls -ld /opt/ar-imms/node-exporter
sudo ss -ltnp '( sport = :9100 )'
```

Only after confirming it is stale AR-IMMS test residue, remove it:

```bash
sudo systemctl disable --now ar-imms-node-exporter.service
sudo rm -f /etc/systemd/system/ar-imms-node-exporter.service
sudo systemctl daemon-reload
sudo rm -rf /opt/ar-imms/node-exporter
```
