# Context boundaries and public contracts

**Status:** DESIGN BASELINE / not implemented. Distinguish state owner, application command and external adapter. Avoid identical directory scaffolding where there is no complexity.

| Context | Owns | Consumes/emits | Forbidden responsibility |
|---|---|---|---|
| Identity & Connectivity | Stable installation ID, binding, credentials, Core OpAMP session | Enrollment Core client, OpAMP config/status DTO | Monitoring policy decisions, direct Collector writes |
| Configuration & Policy | Desired configuration, profile ownership (policy OPEN), CAS revisions, materialization and reconciliation intent | Monitoring intent, `DesiredConfigurationChanged`; calls Runtime apply port | Process supervision, scraping, arbitrary Core YAML |
| Resource Inventory | Provider-scoped observations, source IDs, provenance, normalized identities/history, completeness | Docker listener, future process/K8s/device providers; `InventorySnapshotCommitted` | Receiver creation, probing secrets, assumption that IP == identity |
| Monitoring Capabilities | Capability catalog, metadata-to-capability matching, supported profile templates, explain status | Inventory projection, pinned Collector manifest; monitoring intents | Native scrape loops or client-specific receiver plugins |
| Runtime & Installation | Pinned Collector binary/manifests, config validation, OS dependencies, activation journal, LKG, supervisor | ApplyCandidate port; `GenerationApplied/Failed` | Core policy ownership or cross-context SQL shortcuts |
| Health & Reporting | Snapshot/status projections, internal diagnostics, OpAMP reporting data | Runtime and Agent evidence | Claiming export delivery from readiness only |
| Remote Operations | Typed operation specs, target binding, execution state, audit/outcome | Core-authorized requests; Docker/OS adapter | SSH, arbitrary shell, OpAMP restart misuse |

## Discovery Provider / Listener contract (proposed)

`ProviderID + Scope + Snapshot(ctx)` returns an **atomic complete** set of `Observation` values or explicit error. `Observation` includes `source_native_id`, `resource_kind`, `observed_at`, `attributes + provenance`, and candidate `endpoints + network context`. A failed/incomplete scan is never an empty successful snapshot. Event-driven notifications may trigger scans but do not replace durable reconciliation. Names and raw IPs are not stable identities. Cross-provider correlation must be evidence-based; do not merge Kubernetes and Docker records solely on port/image/name.

## Orchestration flow

1. Listener snapshot commits to Inventory.
2. Monitoring matches **pre-authorized supported capabilities**, produces explanation; **no per-container prompt**.
3. Configuration adjusts *capability rules/templates* if needed; instance churn stays inside OTel Observer/Receiver Creator.
4. Runtime validates config and applies a generation only when config fingerprint changes.
5. OTel executes receivers, processors, exporters; Health reports evidence.

## Single owner constraint

The legacy Agent compiler materialization path and OTel Receiver Creator must not both instantiate the same receiver/resource. A feature-flagged migration must switch ownership per target and prove parity. Existing proposal/approval records/CLI retained as migration compatibility until explicit deprecation, not part of vNext routine DX.

## Unresolved boundaries

Core-vs-local config authority; precise identity mapping to OpAMP instance UID; credential attachment to protected receivers; privileged operations executor. These require ADRs prior to the relevant PR.
