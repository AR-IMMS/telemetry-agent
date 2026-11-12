# Discovery and monitoring contracts

**Status:** APPROVED FOR PR-05 IMPLEMENTATION
**Release target:** `v0.1.4a-next`

## Identity

| Field | Owner / meaning | Stability |
|---|---|---|
| `node_id` | Core-bound Node identity from the Agent's registered enrollment result | Stable for the active Core binding; Agent does not infer or replace it |
| `resource_id` | Provider-scoped runtime identity; Docker uses the container ID | Stable only for one runtime resource; changes on container recreation |
| `workload_id` | Optional logical identity supplied by the workload label `ar-imms.workload.id` | Stable across recreation only when the workload operator keeps the label value stable |
| container name | Display metadata | Mutable and never an authoritative identity |

Docker monitoring opt-in uses these labels:

```yaml
labels:
  ar-imms.monitoring.enabled: "true"
  ar-imms.monitoring.profile: "redis"
  ar-imms.workload.id: "cache-service" # optional
```

The Agent inventory retains the provider key `(provider, provider_instance, resource_id)` and the optional workload label separately. Missing or duplicated `workload_id` values do not merge resources; container ID remains the runtime key.

## Discovery and profile contract

A provider returns a normalized, provider-scoped snapshot containing runtime identity, display metadata, allowlisted attributes, endpoint evidence, observation time, and an explicit completeness result. A provider error, invalid snapshot, or incomplete snapshot records diagnostics and does not mark prior resources missing. Discovered endpoints are evidence, not a reachability assertion.

A monitoring profile declares its profile ID, provider match, required labels, required pinned Collector capabilities, endpoint strategy by platform/provider, and receiver configuration. PR-05 ships a small built-in Docker/Redis profile. A profile registry is a composition list with validation; it is not a dynamic plugin loader. New providers and profiles extend that registry without changing Docker snapshot normalization or reconciliation rules.

## Ownership and activation

- Agent owns inventory, the profile registry and opt-in policy, rendered configuration, Collector validation/activation, generation tracking, and last-known-good recovery.
- OTel `docker_observer` observes the configured local Docker Engine. OTel `receiver_creator` owns the Redis receiver instance lifecycle for containers matching the Redis profile labels.
- For a target selected by the native profile, Agent resolution omits its legacy Redis receiver contribution. Exactly one receiver owner is active for that target.
- Unlabelled Redis targets and unbound/manual Agent Redis integrations retain the existing Agent-managed receiver path. Removing native profile configuration makes bound targets eligible for the legacy path again.
- Receiver rules contain profile policy and Node identity, not per-container addresses. Add/remove/recreate changes OTel receiver instances and Agent inventory, not the Collector configuration generation.
- The native profile is composed only when Docker discovery has an explicit valid local endpoint and a registered Core `node_id` is available. Until both are available, current Agent behavior remains active.
- A rejected or unhealthy configuration continues through the existing candidate activation and LKG path. No SQLite schema change is introduced by PR-05.

## Endpoint and telemetry identity

- The initial Redis profile supports private container port `6379` only; all generated Receiver Creator rules require `port == 6379`. Other exposed/published ports do not select a Redis receiver.
- Windows Docker Desktop uses its host-published `alternate_port` through `127.0.0.1`; rules do not guess a container bridge address. A nondefault host-mapped port is supported while the private Redis port remains `6379`.
- Linux first uses the observed Docker endpoint when no host-mapped `alternate_port` is present, and prefers the mapped loopback port when it is present. This accommodates native Linux networking and the verified WSL2/Docker Desktop routing case.
- Inventory `inspect` reports the matched profile and classifies endpoint inventory evidence as `endpoint_missing`, `endpoint_ambiguous`, or `endpoint_resolved`. `endpoint_resolved` explicitly means reachability is unverified. Missing and ambiguous evidence fail closed; inventory alone never labels an endpoint unreachable.
- `endpoint_unreachable` requires an OTel metrics scrape error correlated to the profile receiver rule and the resolved endpoint. The pinned Collector v0.160.0 Receiver Creator log identifies dynamic receiver failures through its `name` field while `otelcol.component.id` remains `receiver_creator`; the Agent parser handles that shape. **Open PR-05 gap:** runtime Collector output is not yet routed into persistent/local profile diagnostics, so the CLI cannot currently report `endpoint_unreachable` from live OTel evidence.
- Docker Observer can emit one endpoint observation per network for a container. The Windows live log showed repeated Receiver Creator start records for the same endpoint ID before a stop record while network observations changed. Delivery succeeded, but the logs do not establish one active OTel receiver per target or rule out duplicate points. Multi-network receiver uniqueness remains an open PR-05 gate.
- Receiver resource attributes include Core `node_id`, provider `docker`, container `resource_id`, optional `workload_id`, and container name as display metadata. `container.id` remains available for correlation. Separate instances stay distinguishable even without `workload_id`.
- Status evidence has four separate meanings: `discovered` comes from Agent inventory; `receiver active` requires Receiver Creator start evidence; `collection verified` requires Redis metric points; `delivery verified` requires those points arriving at an isolated OTLP sink. One level does not imply the next.

## Failure semantics

- Docker provider errors/incomplete snapshots retain inventory history and provider error status.
- Docker Observer startup/access failures reject the new candidate or cause Collector startup recovery; the prior LKG configuration remains authoritative.
- An endpoint with no suitable mapping is not guessed. The target can remain discovered while no receiver is active. The inventory CLI reports missing/ambiguous evidence distinctly from an OTel connection failure; live unreachable-state delivery through Agent diagnostics remains open for PR-05.
- A receiver that starts but cannot scrape is `receiver active` without `collection verified`.
- Collected metrics without matching points at the isolated OTLP sink are not `delivery verified`.
