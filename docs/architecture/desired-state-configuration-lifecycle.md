# Desired State and Collector Configuration Activation

## Purpose

This document defines how the agent safely changes managed telemetry dependencies without leaving the OpenTelemetry Collector on a stale or invalid configuration.

It separates user intent, active configuration, and the Collector runtime state.

## Model

The agent persists three generations:

| Field                 | Meaning                                                                                                        |
| --------------------- | -------------------------------------------------------------------------------------------------------------- |
| `DesiredGeneration`   | The latest dependency state requested by the user.                                                             |
| `ActivatedGeneration` | A valid Collector configuration for that desired state has been rendered, validated, and atomically activated. |
| `AppliedGeneration`   | The Collector is healthy while running with the activated configuration.                                       |

A dependency is enabled or disabled in the desired state. The rendered Collector configuration contains receivers only for enabled dependencies supported by the current platform.

## Activation Flow

<!-- Paste the lifecycle sequence diagram here. -->

For an enable operation:

1. The dependency installer creates or reconciles the dependency.
2. The agent records the dependency as enabled and advances `DesiredGeneration`.
3. The lifecycle coordinator renders a Collector configuration from the new desired state.
4. The Collector validates the candidate configuration.
5. The agent atomically activates the configuration and records `ActivatedGeneration`.
6. The runtime watcher restarts the Collector and waits for its health check.
7. When healthy, the agent records `AppliedGeneration`.

## Safety Rules

- A configuration is never replaced before Collector validation succeeds.
- `ActivatedGeneration` is updated only after the new configuration is active.
- The runtime watcher reacts to `ActivatedGeneration`, never directly to `DesiredGeneration`.
- Disable and uninstall first remove the dependency receiver from the active Collector configuration.
- Resource teardown happens only after the Collector has applied the configuration that no longer references the dependency.
- Ownership records determine which resources the agent may remove. Unknown or third-party resources are never removed.

## Failure and Recovery

If installation succeeds but configuration activation fails, the dependency may exist on the host, but the Collector continues using its previous valid configuration. The desired generation remains ahead of the activated generation.

If the process stops during an operation, persisted generations show which step remains incomplete. A later invocation can reconcile the state without guessing whether a configuration was activated or applied.

## Scope

This lifecycle is local to one machine. It does not introduce a remote control plane, remote command execution, or central dependency orchestration.
