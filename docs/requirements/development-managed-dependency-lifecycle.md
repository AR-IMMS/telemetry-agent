# Managed Dependency Lifecycle

## Goal

Provide a consistent lifecycle interface for telemetry dependencies managed by
`agentctl`.

The initial integrations are:

- Windows Exporter
- Node Exporter
- Libre Hardware Monitor

## User Interface

```text
agentctl dependency list
agentctl dependency status
agentctl dependency install [dependency-name]
agentctl dependency configure [--state-path <path>]
agentctl dependency pending [--state-path <path>]
agentctl dependency disable <dependency-name> [--state-path <path>]
agentctl dependency uninstall <dependency-name> [--state-path <path>]
```

`dependency install` without a name opens an interactive terminal picker.

`dependency configure` opens an interactive terminal selector for dependencies
recorded in Agent state. It toggles their desired enabled state; it does not
install previously unmanaged dependencies.

## Required Behaviour

- List only dependencies supported by the current operating system.
- Install by stable dependency name.
- Reconcile an existing managed dependency instead of blindly reinstalling it.
- Show lifecycle status without changing the machine.
- Report health and configuration drift independently.
- Keep catalog ordering deterministic.
- Require every catalog integration to provide both an installer and an
  inspector.
- Require a bootstrapped Collector before changing managed dependency state.
- Enable a disabled dependency only when Agent-owned resources are recorded.
- Render, validate, and activate Collector configuration for every lifecycle
  state change.
- Execute physical disable or uninstall only after the Collector acknowledges
  the activated configuration generation.
- Preserve dependency state and ownership after disable.
- Remove the dependency record only after uninstall teardown completes.
- Make `dependency configure` a no-op for entries whose desired state did not
  change.
- Make terminal cancellation leave lifecycle state and host resources unchanged.

## Lifecycle Status

A dependency status contains:

- **Availability:** `enabled`, `disabled`, or `unknown`
- **Health:** `healthy`, `unhealthy`, or `unknown`
- **Drift:** whether Agent-owned configuration differs from the expected state

Examples:

```text
windows-exporter: enabled (healthy)
windows-exporter: enabled (unhealthy, drifted)
node-exporter: disabled (unknown)
```

An inspection failure is reported as `unknown` with its diagnostic message.

## Safety Rules

- Reuse a healthy dependency only when its managed configuration matches.
- Repair managed configuration drift only when the integration can do so safely.
- Do not overwrite an unhealthy existing dependency when ownership or recovery
  safety is unknown.
- Validate Administrator privileges before machine-wide Windows changes.
- Keep inspection read-only.
- Do not physically remove an unowned or third-party dependency.
- Do not stop the Collector runtime while dependency teardown is pending.

## Non-Goals

This increment does not provide:

- Agent-wide uninstall or local data purge
- Remote fleet management
- A central Operations Controller
- Automatic repair of arbitrary third-party installations
- Automatic ownership adoption for existing dependencies
