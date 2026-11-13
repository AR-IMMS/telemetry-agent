# Context boundaries and public contracts

**Status:** DESIGN BASELINE / not implemented. Distinguish state owner, application command and external adapter. Avoid identical directory scaffolding where there is no complexity.

## Current release-mission terminology

The current `v0.1.4a-next` release mission is authoritative for the business
context count and names: Identity, Inventory, Monitoring, Configuration,
Runtime, Health, and Operations. The group labels below are earlier descriptive
aliases: Identity & Connectivity → Identity; Resource Inventory → Inventory;
Monitoring Capabilities → Monitoring; Configuration & Policy → Configuration;
Runtime & Installation → Runtime; Health & Reporting → Health; Remote
Operations → Operations. The OpAMP Server is a separate central-management
service boundary, not an eighth Agent business context. This terminology note
does not mandate a package layout; PR-09.5 will reconcile detailed ownership
from actual source before moving code.

| Context | Owns | Consumes/emits | Forbidden responsibility |
|---|---|---|---|
| Identity & Connectivity | Stable installation ID, binding, credentials, Agent-to-Core business API session | Enrollment Core client, installation identity contract | Monitoring policy decisions, direct Collector writes, OpAMP instance authentication by self-report |
| Configuration & Policy | Desired configuration, profile ownership (policy OPEN), CAS revisions, materialization and reconciliation intent | Monitoring intent, `DesiredConfigurationChanged`; calls Runtime apply port | Process supervision, scraping, arbitrary Core YAML |
| Resource Inventory | Provider-scoped observations, source IDs, provenance, normalized identities/history, completeness | Docker listener, future process/K8s/device providers; `InventorySnapshotCommitted` | Receiver creation, probing secrets, assumption that IP == identity |
| Monitoring Capabilities | Capability catalog, metadata-to-capability matching, supported profile templates, explain status | Inventory projection, pinned Collector manifest; monitoring intents | Native scrape loops or client-specific receiver plugins |
| Runtime & Installation | Pinned Collector and (after parity) OpAMP Supervisor artifacts, config validation, OS dependencies, activation/LKG evidence, one lifecycle owner | ApplyCandidate port during migration; Supervisor provisioning/status evidence | Core policy ownership or concurrent Collector lifecycle owners |
| OpAMP Adapter (server side) | Native OpAMP protocol session and translation to Core-owned contracts | Core auth, identity binding, desired policy, status/health APIs | Durable policy authority, mock-only calls, Collector business rules |
| Health & Reporting | Snapshot/status projections, internal diagnostics, evidence from Agent, Supervisor, Collector and OTLP sink | Runtime and Agent evidence; supported OpAMP status | Claiming collection/delivery from readiness or protocol status alone |
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

PR-05's approved Docker/Redis identity, label opt-in, endpoint strategy, ownership, rollback, and evidence semantics are specified in [discovery-monitoring-contracts.md](discovery-monitoring-contracts.md). That contract assigns dynamic receiver lifecycle to OTel only for opted-in Docker targets while preserving Agent-managed Redis receivers for the legacy path.

## OpAMP migration boundary

Existing `internal/supervisor` remains the current Agent Collector process
supervisor and is distinct from OTel OpAMP Supervisor. Keep it and
`internal/agentlifecycle` authoritative until exact-version parity is proven;
transfer process ownership once, with reversible fallback. OTel Supervisor owns
Collector process lifecycle after transfer, while the Agent retains inventory,
monitoring policy, local configuration generation, dependency provisioning,
and product diagnostics. See [OpAMP integration](opamp-integration.md).

## Unresolved boundaries

Core-vs-local config authority; precise identity mapping to OpAMP instance UID; credential attachment to protected receivers; privileged operations executor. These require ADRs prior to the relevant PR.
