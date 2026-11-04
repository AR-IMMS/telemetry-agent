# 2026-10-01: Agent Clean-Uninstall E2E Checkpoint

## Summary

Validated the Linux end-to-end lifecycle for the `feat/agent-clean-uninstall` branch using Node Exporter as a managed dependency.

The verified sequence was:

```text
Agent install
→ Collector bootstrap
→ Node Exporter install
→ Node Exporter disable
→ Node Exporter uninstall
→ Agent uninstall
```

## Verified Behavior

### Managed dependency lifecycle

- Node Exporter was installed as an Agent-managed dependency.
- Disabling Node Exporter stopped the runtime and closed port `127.0.0.1:9100`.
- Dependency uninstall removed:
  - `ar-imms-node-exporter.service`
  - `/opt/ar-imms/node-exporter`
  - the dependency entry from Agent state
- Final dependency state was:

```json
{
  "dependencies": {}
}
```

- Generation tracking converged after uninstall:

```text
desiredGeneration = activatedGeneration = appliedGeneration
```

### Agent clean uninstall

Agent uninstall was invoked from the external E2E binary, not from the installation being removed.

The operation removed:

- `ar-imms-telemetry-agent.service`
- `/opt/ar-imms/telemetry-agent`
- `/var/lib/ar-imms/telemetry-agent`

The operation preserved repository-owned source configuration:

```text
configs/
├── base/
├── os/
├── profiles/
└── spikes/
```

## Investigation Note

An earlier test environment contained stale Node Exporter resources from a prior run:

- inactive `ar-imms-node-exporter.service`
- `/opt/ar-imms/node-exporter`
- a prior Node Exporter process history

That residue caused a later install to refuse to overwrite an unhealthy existing service. After explicit cleanup and a fresh E2E run, dependency uninstall removed the service unit, installation directory, runtime endpoint, and persisted dependency state correctly.

Therefore, the fresh E2E evidence does not show a dependency teardown defect in the current branch.

## Open Follow-up

The E2E run exposed command and configuration consistency gaps:

1. `agentctl` subcommands do not share a canonical default state path.
2. `agentctl install` does not make a Collector immediately ready for dependency management.
3. The installed `agentctl` binary is not available through a stable shell path.

These are documented in [Agentctl Command and State Consistency Gaps](../architecture/agentctl-command-state-consistency.md).
