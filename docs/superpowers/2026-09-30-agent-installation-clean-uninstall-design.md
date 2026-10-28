# Agent Installation and Clean Uninstall Design

## Status

Proposed

## Goal

Provide an installed production mode for `agentctl` on Windows and Linux, then allow a safe full uninstall that removes only resources created and recorded by the Agent.

The feature supports:

- Windows Service on Windows.
- systemd service on Linux.
- Agent-owned Collector installation and rendered configuration.
- Agent-owned managed dependency teardown.
- Idempotent reruns after interrupted uninstall.

## Non-Goals

This feature does not:

- Remove manually installed Collectors or dependencies.
- Remove user-provided `config-root` files.
- Support Linux init systems other than systemd.
- Embed default configuration files into the Agent binary.
- Support `go run ... install`; installed mode requires a durable release binary.
- Remove Windows Event Log or systemd journal entries.

## Installed Mode

`agentctl install --config-root <path> [gateway options]` installs the Agent.

The command:

1. Selects the platform-specific default layout.
2. Copies the current release binary into an Agent-owned binary directory.
3. Uses the existing bootstrap flow to install the Collector into an Agent-owned directory and render its active configuration into Agent-owned data storage.
4. Persists the Agent state in Agent-owned data storage.
5. Records the exact resources created by the installation.
6. Creates and starts the Agent service, which executes:

   ```text
   agentctl run --state-path <agent-owned state path>
   ```

Existing `agentctl bootstrap` remains available for development and portable operation. Portable bootstrap does not create an Agent installation ownership record and is not eligible for full Agent uninstall.

## Platform Layouts

The defaults are supplied by platform-specific layout implementations, not scattered through command handlers.

| Platform | Binary root                                     | Data root                                     | Service manager                 |
| -------- | ----------------------------------------------- | --------------------------------------------- | ------------------------------- |
| Windows  | Program Files AR-IMMS Telemetry Agent directory | ProgramData AR-IMMS Telemetry Agent directory | Windows Service Control Manager |
| Linux    | `/opt/ar-imms/telemetry-agent`                  | `/var/lib/ar-imms/telemetry-agent`            | systemd                         |

The actual resolved paths and service name are persisted as ownership resources. Uninstall never reconstructs paths from defaults.

## Persistent Ownership

`agentstate.State` gains an Agent installation record that reuses `OwnershipRecord` and `OwnedResource`.

It records only concrete resources created by installed mode:

- Agent service or systemd unit.
- Copied Agent executable.
- Agent-owned data directory and lifecycle state.
- Fresh Agent-owned Collector installation directory.
- Generated Collector configuration file.

`config-root` is always user-owned. A reused Collector installation is not recorded as Agent-owned unless an existing ownership record already proves ownership.

## Service Boundary

A platform-neutral service contract is used by shared installation orchestration.

```text
install service
stop service
remove service
```

Windows and Linux implementations are isolated with platform files and build tags. They follow the existing repository patterns:

- injected `systemctl` command execution for Linux;
- isolated Windows SCM interaction for Windows.

Dependency-specific service packages are not imported by Agent installation code.

## Clean Uninstall

`agentctl uninstall --yes` is available only when an Agent installation ownership record exists.

The command performs the following order:

1. Verify explicit confirmation and Agent installation ownership.
2. Reuse the existing dependency lifecycle to request uninstall of every Agent-owned dependency.
3. Wait through the local health endpoint until the Collector is ready and pending dependency teardowns equal zero.
4. Stop and remove the Agent service through the platform adapter.
5. Remove only recorded Agent-owned Collector, Agent binary, generated configuration, data, and state resources.
6. Remove the ownership record as part of deleting Agent-owned state.

Windows uses a short-lived cleanup helper copied to a temporary Agent-owned location. The helper waits for the invoking `agentctl` process to exit before removing the installed executable and final directories. Linux may remove an unlinked executable directly but uses the same cleanup contract for consistent behavior.

## Failure and Retry Rules

- No Agent-owned installation resource is removed while any dependency teardown is pending.
- If Collector health is unavailable or degraded during the wait, uninstall stops and leaves state intact.
- If service stop or removal fails, filesystem cleanup does not begin.
- If cleanup is interrupted, the remaining ownership record permits a later rerun.
- Unknown, missing, or manually created resources are skipped rather than deleted.
- Every destructive command requires administrator privileges on Windows or root-equivalent privileges on Linux.

## Package Boundaries

```text
internal/agentstate
  Persistent Agent installation ownership.

internal/agentinstallation
  Shared layout, install, uninstall orchestration, and platform adapters.

internal/agentlifecycle
  Existing configuration activation and dependency teardown safety gate.

internal/agenthealth
  Existing live status endpoint used while waiting for teardown completion.

cmd/agentctl
  install and uninstall command parsing plus production dependency wiring.
```

## Testing Strategy

Tests do not create real services or delete real host files.

- State tests verify ownership recording and safe mutation rules.
- Shared installation tests use fake service and filesystem adapters.
- Windows tests verify SCM adapter command/resource behavior through injected seams.
- Linux tests verify systemd command ordering through injected runners.
- Uninstall tests verify schedule, wait, stop, remove, and failure boundaries.
- CLI tests verify confirmation, unsupported portable state, and clear error output.
- End-to-end tests use a temporary layout and fake platform adapters.
