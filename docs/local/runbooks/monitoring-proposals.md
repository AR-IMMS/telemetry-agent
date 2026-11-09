# Monitoring Proposal Operations

Discovery creates reviewable monitoring proposals; it never approves or activates them automatically. Approval persists a desired Integration instance. The Agent Service remains the only S3 reconciliation owner and applies approved state through the existing Collector lifecycle.

## Prerequisites

- Enable Docker discovery on the Agent Service using the local endpoint documented in [Docker Resource Discovery](docker-resource-discovery.md).
- Run `agentctl` under an account that can read the Agent's state database and request the operation.
- Treat Docker access and state database access as privileged. Do not put credentials in command arguments or proposal inputs.
- Use the same state database as the Agent Service. For a non-default database, pass `--state-path` to every command.

## Review

List proposals (JSON output):

```text
agentctl discovery proposals list [--state-path <path>]
```

Inspect one proposal by its stable ID:

```text
agentctl discovery proposals inspect --id <proposal-id> [--state-path <path>]
```

Review the resource identity, exact Integration definition version, resolution/capability state, required inputs, evidence and current proposal revision. A `needs_input`, stale, unsupported or unverified proposal is not ready for approval. Discovered endpoint data is evidence and does not itself prove reachability. Obtain the current desired-set revision with `agentctl integration list`.

## Dismiss or approve

Dismiss using the proposal revision displayed by `list` or `inspect`:

```text
agentctl discovery proposals dismiss --id <proposal-id> --revision <proposal-revision> [--state-path <path>]
```

Approve using both the current proposal revision and desired Integration set revision. Obtain the desired-set revision from `agentctl integration list`. Supply only non-secret missing inputs:

```text
agentctl discovery proposals approve --id <proposal-id> --revision <proposal-revision> --expected-desired-revision <desired-set-revision> [--input name=value,name=value] [--state-path <path>]
```

Approval revalidates current inventory and matching evidence and atomically commits the desired instance with the terminal proposal outcome. A stale revision or changed resource evidence fails closed; inspect the proposal again before retrying. Do not repeat approval with old revision values.

The command reports that Agent Service convergence is pending. It does not start a one-shot reconciliation or claim Collector readiness or telemetry delivery. Check the Agent's reconciliation status/history for application outcomes and separately verify exported telemetry when delivery must be established.

## Security and limitations

- Never pass passwords, tokens, connection strings containing credentials, or secret references through `--input`; this CLI accepts only non-secret settings.
- Proposal evidence does not copy Docker environment variables or arbitrary labels.
- Credentials for authenticated database integrations require a separately approved secure secret-reference and Collector expansion contract; such proposals remain non-actionable.
- A resource disappearing does not automatically disable an already approved Integration. Inventory freshness/presence and proposal status are distinct from desired state.
- PostgreSQL and MongoDB test fixtures do not imply production Integration definitions or verified Collector pipelines.
