# Discovery and monitoring contracts

**Status:** APPROVED FOR PR-05; PR-06 extension contract recorded below
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

### PR-06 local profile extension contract

- Profile documents are local configuration under `<config-root>/monitoring-profiles/`; they use a versioned, bounded Agent schema and cannot contain arbitrary Collector YAML, expressions, or executable hooks. The Agent translates approved profile fields to pinned OTel components.
- A profile selects a provider, exact opt-in labels, a Collector capability, a profile-owned target port (with optional validated label override), and receiver-specific typed settings. Generic discovery records observations only; it does not know profile IDs or private-port defaults.
- Initial native receiver types are Redis, PostgreSQL, and Prometheus. Prometheus profiles may set a bounded metrics path and interval. Adding another Prometheus endpoint, including a test facilities exporter, is a data-only profile change after the generic typed profile loader is installed.
- PostgreSQL profile files name username/password environment-variable references; they contain no secret values. The Agent omits the PostgreSQL receiver rule until both values are present in the Agent service environment, while `inventory inspect` reports `requires_configuration`. The Collector process resolves references through its normal environment expansion. `tls_ca_file` may name a trusted local CA file; no insecure TLS bypass is exposed. These local references do not define Core/OpAMP secret authority.
- The profile registry rejects unknown schema fields, unsupported receiver/capability combinations, duplicate identities or receiver rules, unsafe path/interval/port values, and a profile that cannot be represented by the pinned binary manifest. A rejected change leaves the active Collector candidate and LKG unchanged.
- Docker Observer and Receiver Creator remain the only dynamic discovery/receiver lifecycle engines. Agent inventory and profile policy determine intent; exactly one owner is selected for each target. Existing non-profile Redis behavior remains compatible.

Open: configuration authority precedence between Core and local files remains unresolved and is not changed by PR-06. The local loader is a release-local composition input, not a remote configuration API.

## Ownership and activation

- Agent owns inventory, the profile registry and opt-in policy, rendered configuration, Collector validation/activation, generation tracking, and last-known-good recovery.
- OTel `docker_observer` observes the configured local Docker Engine. OTel `receiver_creator` owns the Redis receiver instance lifecycle for containers matching the Redis profile labels.
- For a target selected by the native profile, Agent resolution omits its legacy Redis receiver contribution. Exactly one receiver owner is active for that target.
- Unlabelled Redis targets and unbound/manual Agent Redis integrations retain the existing Agent-managed receiver path. Removing native profile configuration makes bound targets eligible for the legacy path again.
- Receiver rules contain profile policy and Node identity, not per-container addresses. Add/remove/recreate changes OTel receiver instances and Agent inventory, not the Collector configuration generation.
- The native profile is composed only when Docker discovery has an explicit valid local endpoint and a registered Core `node_id` is available. Until both are available, current Agent behavior remains active.
- A rejected or unhealthy configuration continues through the existing candidate activation and LKG path. No SQLite schema change is introduced by PR-05.

## Endpoint and telemetry identity

- A Docker monitoring profile declares a default private target port and an optional profile-specific workload label for an override. Redis defaults to `6379`; `ar-imms.monitoring.port` may override it with an ASCII decimal TCP port in `1..65535`. An absent label uses the default. An invalid override is retained in inventory, reported as `endpoint_invalid`, and must not match a Receiver Creator rule. The Docker provider only allowlists and records the label; it does not interpret profile ports.
- The selected private target port is applied consistently to published and container-network endpoint evidence and to the profile's OTel rule. The profile owns this validation and endpoint policy; generic discovery continues to report observed endpoints without choosing a monitoring target.
- Windows Docker Desktop uses a host-published `alternate_port` through `127.0.0.1`; it does not guess a container bridge address. Linux uses the endpoint emitted by the pinned Docker Observer, including its `alternate_port` strategy where the host mapping is usable.
- Docker Observer endpoint selection is deterministic for containers attached to multiple networks: with `use_hostname_if_present: false`, select the network with greatest `GwPriority`, falling back to alphabetical network name when the Docker API does not provide a priority. The shipped profile uses Docker Observer's documented `alternate_port` behavior for host-published ports and the Agent's diagnostic projection uses the same fallback order for unpublished endpoints. If the provider cannot supply network names, multiple container-network candidates remain `endpoint_ambiguous`.
- Inventory `inspect` reports the matched profile and classifies endpoint inventory evidence as `endpoint_missing`, `endpoint_ambiguous`, or `endpoint_resolved`. `endpoint_resolved` explicitly means reachability is unverified. Missing and ambiguous evidence fail closed; inventory alone never labels an endpoint unreachable.
- `endpoint_unreachable` requires an OTel metrics scrape error correlated to the profile receiver rule and selected endpoint. Collector output is observed by the Agent lifecycle and exposed through its local read-only diagnostics as sanitized receiver, endpoint, code, and UTC observation time. `inventory inspect` labels it as the last observed failure; it is not a continuous reachability probe. It does not alter desired configuration or inventory presence, and is not inferred from discovery alone.
- Receiver lifecycle evidence is keyed by Collector `service.instance.id`, profile receiver ID, and Observer `endpoint_id`. A new Collector instance legitimately starts its receivers again; a repeated start for the same key before a stop within one Collector instance is a duplicate candidate and must be checked against received metric points. No inventory churn alone may activate a new Collector generation.
- Receiver resource attributes include Core `node_id`, provider `docker`, container `resource_id`, optional `workload_id`, and container name as display metadata. `container.id` remains available for correlation. Separate instances stay distinguishable even without `workload_id`.
- Status evidence has four separate meanings: `discovered` comes from Agent inventory; `receiver active` requires Receiver Creator start evidence; `collection verified` requires Redis metric points; `delivery verified` requires those points arriving at an isolated OTLP sink. One level does not imply the next.

## Failure semantics

- Docker provider errors/incomplete snapshots retain inventory history and provider error status.
- Docker Observer startup/access failures reject the new candidate or cause Collector startup recovery; the prior LKG configuration remains authoritative.
- An endpoint with no suitable mapping is not guessed. The target can remain discovered while no receiver is active. The diagnostics distinguish missing, invalid override, ambiguous, selected-but-unverified, and OTel-evidenced unreachable states.
- A receiver that starts but cannot scrape is `receiver active` without `collection verified`.
- Collected metrics without matching points at the isolated OTLP sink are not `delivery verified`.
