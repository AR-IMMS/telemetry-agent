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
agentctl dependency install <dependency-name>
agentctl dependency install
```

`dependency install` without a name opens an interactive terminal picker. The
user can select multiple dependencies, then confirm the installation.

## Required Behaviour

- List only dependencies supported by the current operating system.
- Install by stable dependency name.
- Reconcile an existing managed dependency instead of blindly reinstalling it.
- Show lifecycle status without changing the machine.
- Report health and configuration drift independently.
- Keep catalog ordering deterministic.
- Require every catalog integration to provide both an installer and an
  inspector.

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

## Non-Goals

This increment does not provide:

- Dependency uninstall or disable commands
- Remote fleet management
- A central Operations Controller
- Automatic repair of arbitrary third-party installations
