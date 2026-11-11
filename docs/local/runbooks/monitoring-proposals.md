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

## Local automatic monitoring Templates

Use a Template when an operator explicitly authorizes the Agent to create and maintain desired instances for every resource matching its selector. A Template is not a Core policy, and enabling it is broader than approving one proposal. Use a narrow selector and review endpoint binding rules before enabling.

Create a local Redis Template (PowerShell users may save the JSON to a file and pass its contents as one quoted argument):

```text
agentctl integration template create --expected-revision 0 --template-json '{"id":"redis-local","definition_id":"redis","definition_version":"1.0.0","owner":"local","status":"enabled","selector":{"resource_kinds":["container"],"exact_attributes":{"container.image.repository":"docker.io/library/redis"}},"bindings":[{"target":"endpoint","source":{"kind":"endpoint","endpoint_scope":"host","endpoint_protocol":"tcp","endpoint_provenance":"published_port"}}]}' [--state-path <path>]
```

List and inspect Templates:

```text
agentctl integration template list [--state-path <path>]
agentctl integration template inspect --id redis-local [--state-path <path>]
```

Update the full Template JSON using its displayed per-Template revision as the CAS token. Enable/disable are also revision-checked. Disable suspends only the Template-owned generated instances and can be reversed; delete is a permanent tombstone and cannot be re-enabled or reused:

```text
agentctl integration template update --expected-revision <revision> --template-json <json> [--state-path <path>]
agentctl integration template disable --id redis-local --expected-revision <revision> [--state-path <path>]
agentctl integration template enable --id redis-local --expected-revision <revision> [--state-path <path>]
agentctl integration template delete --id redis-local --expected-revision <revision> [--state-path <path>]
```

The Agent Service materializes changes after a complete successful Discovery provider cycle. Repeated unchanged projections do not advance the desired-set revision. Provider failure does not suspend instances; confirmed absence follows the existing two-complete-snapshot policy. Check Integration desired state and S3 reconciliation status separately. These commands do not run reconciliation or claim activation/telemetry delivery.

Templates and Integration settings reject plaintext credentials. Candidates requiring unavailable inputs, ambiguous endpoints, conflicting Templates or unsupported Collector capabilities are not materialized automatically; use the proposal workflow where it is eligible, otherwise leave the Integration disabled until its requirements are resolved.
