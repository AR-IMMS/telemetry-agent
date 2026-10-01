# Agentctl Command and State Consistency Gaps

## Status

Open follow-up identified during the 2026-10-01 clean-uninstall E2E validation.

This document describes command ergonomics and state-location consistency. It does not change the ownership-based cleanup model or the dependency lifecycle ordering.

## Observed Current Behavior

The Agent installation command persists state at:

```text
/var/lib/ar-imms/telemetry-agent/state.json
```

However, `bootstrap` and dependency commands use another default state location when `--state-path` is omitted:

```text
/var/lib/ar-imms/agent/state.json
```

This creates an inconsistent flow:

```text
agentctl install
  → state in /var/lib/ar-imms/telemetry-agent/state.json

agentctl bootstrap
  → default lookup in /var/lib/ar-imms/agent/state.json

agentctl dependency install
  → default lookup in /var/lib/ar-imms/agent/state.json
```

As a result, a correctly bootstrapped Collector can appear absent to dependency management unless every command is given an explicit `--state-path`.

Additionally, `agentctl install --config-root <path>` installs the Agent service but does not complete the Collector bootstrap required before a dependency can be installed.

The installed binary is located at:

```text
/opt/ar-imms/telemetry-agent/agentctl
```

It is not available as `agentctl` in a newly opened shell unless the user changes their `PATH` or uses the absolute path.

## Impact

- First-time installation requires users to know internal storage and Collector paths.
- Commands can read or write different state files without an obvious error.
- Dependency management reports that the Collector is not bootstrapped even after bootstrap succeeded in the Agent installation state.
- Operational runbooks require repeated absolute binary paths.
- These issues make an otherwise valid lifecycle look broken.

## Desired Behavior

All lifecycle commands must use the same canonical default state path:

```text
/var/lib/ar-imms/telemetry-agent/state.json
```

The expected operator flow is:

```text
sudo agentctl install --config-root "$PWD/configs"
  → bootstrap Collector
  → render and validate Collector configuration
  → activate Collector
  → install and start Agent service
  → persist one canonical Agent state

sudo agentctl dependency install node-exporter
  → read the same canonical Agent state
  → manage the dependency
```

A user should not need to supply any of the following for a standard installation:

- `--state-path`
- Collector install directory
- Collector config path
- absolute `agentctl` binary path

## Recommended Changes

### 1. Canonicalize default state configuration

Define one shared default state path in the command/configuration layer and use it for:

- `install`
- `bootstrap`
- `dependency install`
- `dependency enable`
- `dependency disable`
- `dependency uninstall`
- `dependency status`
- `uninstall`
- Agent service runtime startup

An explicit `--state-path` remains available only for tests and advanced deployments.

### 2. Make installation operationally complete

`agentctl install --config-root <path>` should bootstrap the Collector before reporting successful Agent installation.

If bootstrap remains a separate command, `install` must clearly state that the Agent is not ready for dependency management and print the exact next command with all required values. Automatic bootstrap is preferred because it matches the operator expectation of “install”.

### 3. Provide a stable executable path

Install a stable system command, for example:

```text
/usr/local/bin/agentctl
```

It may be a symlink to the Agent-owned binary. The path must be recorded as an owned resource and removed during Agent uninstall.

Agent uninstall must still reject execution from a binary inside its own installation directory. An external binary or separately managed launcher remains required for destructive self-removal.

### 4. Bound status checks

`dependency status` should have a command timeout when inspecting systemd and runtime endpoints. A canceled command must be reported distinctly from an unhealthy dependency.

After a dependency is disabled, status presentation should reflect both desired lifecycle state and runtime state, for example:

```text
node-exporter:
  desired: disabled
  runtime: absent
```

## Acceptance Criteria

- Running `agentctl install --config-root "$PWD/configs"` produces a Collector-ready Agent state.
- `agentctl dependency install node-exporter` works without `--state-path` after standard installation.
- Every standard command reads `/var/lib/ar-imms/telemetry-agent/state.json`.
- `agentctl` is available through a stable installed command path.
- A disabled dependency is not presented as healthy.
- Status inspection completes within a defined timeout or returns a clear timeout error.
- Existing explicit-path E2E scenarios continue to pass.
