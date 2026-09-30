# Agent Installation Runbook

## Purpose

Use this runbook to install and operate `agentctl` as the persistent local
Telemetry Agent service.

Installation copies a durable Agent binary, bootstraps a pinned OpenTelemetry
Collector, records Agent ownership, and starts an operating-system service.

## Prerequisites

- Linux amd64 or Windows amd64.
- A valid Collector configuration root containing:

  ```text
  base/otel.yaml
  profiles/laptop.yaml
  os/<linux-or-windows>/otel.yaml
  ```

- A reachable central OTLP gateway endpoint.
- Internet access when the pinned Collector artifact has not been downloaded.
- Elevated privileges: `sudo` on Linux or Administrator PowerShell on Windows.

The gateway endpoint must be external to the local Collector. Do not use the
Agent's own OTLP receiver address, such as `127.0.0.1:4317`, as the gateway
endpoint.

## Install on Linux

Build the binary:

```bash
go build -o ./bin/agentctl ./cmd/agentctl
```

Install and start the service:

```bash
sudo ./bin/agentctl install \
  --config-root "$PWD/configs" \
  --gateway-endpoint otel-gateway.example.internal:4317
```

The command copies the binary to the Agent-owned directory, validates and
installs the Collector, writes persistent state, and enables then restarts the
systemd service.

Verify:

```bash
./bin/agentctl status
sudo systemctl is-enabled ar-imms-telemetry-agent
sudo systemctl status ar-imms-telemetry-agent --no-pager
```

Expected result:

```text
Agent health: healthy
Collector state: ready
```

## Install on Windows

From an Administrator PowerShell:

```powershell
.\agentctl.exe install `
  --config-root C:\AR-IMMS\Telemetry-Agent\Config `
  --gateway-endpoint otel-gateway.example.internal:4317
```

Verify:

```powershell
.\agentctl.exe status
Get-Service ar-imms-telemetry-agent
```

## Installed Resources

| Platform | Service | Agent binary | Persistent data |
| --- | --- | --- | --- |
| Linux | `ar-imms-telemetry-agent.service` | `/opt/ar-imms/telemetry-agent/agentctl` | `/var/lib/ar-imms/telemetry-agent/` |
| Windows | `ar-imms-telemetry-agent` | `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe` | `C:\ProgramData\AR-IMMS\Telemetry Agent\` |

Persistent data contains the rendered Collector configuration, Collector
artifact, and Agent lifecycle state.

The `--config-root` directory belongs to the operator. The Agent reads it but
does not record ownership of it and must not remove it.

## Health and Delivery

```bash
agentctl status
```

`healthy` means the Agent service is running and the local Collector has reached
its ready state with the expected configuration generation.

It does not prove that the central gateway received telemetry. If the gateway is
unreachable, the Collector remains healthy but logs export retry warnings.

Inspect service logs when delivery is in doubt:

```bash
sudo journalctl -u ar-imms-telemetry-agent -n 100 --no-pager
```

A connection refusal to the configured gateway means the destination is not
reachable. Correct the gateway address, network route, or gateway service before
expecting telemetry delivery.

## Routine Service Operations

```bash
sudo systemctl status ar-imms-telemetry-agent
sudo systemctl restart ar-imms-telemetry-agent
sudo systemctl stop ar-imms-telemetry-agent
sudo systemctl start ar-imms-telemetry-agent
```

Stopping the service is reversible. It preserves the installed binary, Collector
files, lifecycle state, and dependency ownership records.

Do not run `agentctl run` manually with the same state path while the service is
running.

## Upgrade or Reconfigure

Build the desired Agent binary, then run `install` again with the same
configuration root and gateway endpoint:

```bash
sudo ./bin/agentctl install \
  --config-root "$PWD/configs" \
  --gateway-endpoint otel-gateway.example.internal:4317
```

The service is restarted with the copied binary. Check `agentctl status` and
the systemd journal after every upgrade.

## Dependency Lifecycle

After the Agent is healthy, use `agentctl dependency` commands to manage host
telemetry sources.

The Agent service must remain running for pending dependency disable or
uninstall teardowns to complete safely. See
[Managed Dependencies](managed-dependencies.md).

## Ownership Boundary

The Agent records ownership of its service, installed binary, installation
directory, and persistent data directory.

It does not remove:

- The operator-provided configuration root.
- Dependencies discovered but not installed by the Agent.
- Unrelated services, files, firewall rules, or user data.

Whole-Agent clean uninstall is not available yet. Until it exists, do not
manually delete Agent-owned resources while dependency teardown is pending.

## Foundation Checks

```bash
go test ./... -count=1
go vet ./...
go build ./...
git diff --check
```
