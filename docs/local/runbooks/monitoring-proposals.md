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

## Shipped Integration catalog

List the Definitions available in this Agent build or inspect one exact version:

```text
agentctl integration catalog list
agentctl integration catalog inspect --id redis --version 1.0.0
agentctl integration catalog inspect --id redis --version 1.0.0 --output json
```

An unknown Definition such as Grafana is reported as not shipped and points to the [Integration Developer Guide](../guides/integration-developer-guide.md). A Collector component appearing in capability evidence does not by itself prove a service endpoint is compatible or telemetry is delivered.

## Local automatic monitoring Templates

Use a Template when an operator explicitly authorizes the Agent to create and maintain desired instances for every resource matching its selector. A Template is not a Core policy, and enabling it is broader than approving one proposal. Use a narrow selector and review endpoint binding rules before enabling.

Create a local Redis Template from a normal JSON file. The complete example is [redis-monitoring.json](../examples/redis-monitoring.json):

```text
cp docs/local/examples/redis-monitoring.json redis-monitoring.json
agentctl integration template create --file redis-monitoring.json --expected-revision 0 [--state-path <path>]
```

The example matches running containers whose normalized `container.image.tag` is exactly `7.4.2-alpine`, overrides Redis `collection_interval` to `15s`, and binds the Redis `endpoint` from one host-published TCP endpoint. To match multiple Redis versions, remove `container.image.tag` from `selector.exact_attributes`; the remaining kind/state selector matches every running resource recognized by the Redis Definition. Ambiguous or missing endpoint bindings are not materialized.

List, inspect the summary, or export the complete stored object:

```text
agentctl integration template list [--state-path <path>]
agentctl integration template inspect --id redis-monitoring [--state-path <path>]
agentctl integration template inspect --id redis-monitoring --output json [--state-path <path>]
```

Save the exported JSON, edit `collection_interval` to `30s`, then update using the current per-Template revision as the compare-and-swap token:

```text
agentctl integration template update --file redis-monitoring.json --expected-revision <current-revision> [--state-path <path>]
```

The `revision` in the JSON is informational on write; the CLI's `--expected-revision` controls concurrency and the Configuration service assigns the next revision. A stale revision fails without overwriting newer state. Existing `--template-json '<object>'` create/update input remains supported; `--file` and `--template-json` are mutually exclusive. Files are limited to 1 MiB, unknown JSON fields and trailing JSON objects are rejected.

Enable/disable are revision-checked. Disable suspends only the Template-owned generated instances and can be reversed; delete is a permanent tombstone and cannot be re-enabled or reused:

```text
agentctl integration template disable --id redis-monitoring --expected-revision <revision> [--state-path <path>]
agentctl integration template enable --id redis-monitoring --expected-revision <revision> [--state-path <path>]
agentctl integration template delete --id redis-monitoring --expected-revision <revision> [--state-path <path>]
```

Only fields declared by the selected Integration Definition can be configured. For Redis, `endpoint` is required and can be bound to a unique host-published TCP endpoint; `collection_interval` is an optional positive Go duration with default `10s`. Credentials, arbitrary Collector YAML, expressions and unknown settings are rejected. Use `agentctl integration catalog inspect --id redis --version 1.0.0` to see the shipped settings and capability evidence.

The Agent Service materializes changes after a complete successful Discovery provider cycle. Repeated unchanged projections do not advance the desired-set revision. Provider failure does not suspend instances; confirmed absence follows the existing two-complete-snapshot policy. Check Integration desired state and S3 reconciliation status separately. These commands do not run reconciliation or claim activation/telemetry delivery.

For Docker lifecycle checks, `agentctl inventory list` retains missing resources as history. Use `agentctl inventory list --active-only` to view only resources present in the latest complete provider snapshots. A container absent from one successful snapshot remains present during the safety grace period; a second consecutive successful absence marks it missing and suspends its Template-generated instance. The current Docker poll interval is 30 seconds, and Collector convergence also depends on the reconciliation poll and lifecycle readiness. A Docker provider error is not an absence and preserves existing monitoring.

Templates and Integration settings reject plaintext credentials. Candidates requiring unavailable inputs, ambiguous endpoints, conflicting Templates or unsupported Collector capabilities are not materialized automatically; use the proposal workflow where it is eligible, otherwise leave the Integration disabled until its requirements are resolved.
