# Context boundaries and public contracts

**Source ownership status:** This document defines logical target responsibilities,
not verified source placement. For the audited merged tree and current package
ownership, use the [PR-09.5 source audit](consolidation/01-current-source-audit.md)
and [ownership map](consolidation/02-context-ownership.md). If a package mapping
below conflicts with those source-grounded findings, treat the old mapping as a
historical proposal until architecture approval; do not move code mechanically.

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
| Configuration & Policy | Desired state, approved Core/local authority and revisions, proposal lifecycle/approval, templates, materialization and reconciliation decisions | Inventory evidence, Monitoring recommendations, `DesiredConfigurationChanged`; requests Runtime apply | Profile matching, process supervision, scraping, arbitrary Core YAML |
| Resource Inventory | Provider-scoped observations, source IDs, provenance, normalized identities/history, completeness | Docker listener, future process/K8s/device providers; `InventorySnapshotCommitted` | Receiver creation, probing secrets, assumption that IP == identity |
| Monitoring Capabilities | Profile definitions/matching, metadata recommendations, endpoint diagnostics and monitoring intent | Inventory projection and a narrow query to Runtime's pinned Collector capability catalog | Owning or duplicating the canonical capability catalog, native scrape loops or client-specific receiver plugins |
| Runtime & Installation | Pinned Collector and verified OpAMP Supervisor artifacts, config validation, OS dependencies, activation/LKG evidence, one lifecycle owner | ApplyCandidate port during migration; Supervisor provisioning/status evidence | Core policy ownership or concurrent Collector lifecycle owners |
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

The legacy Agent compiler materialization path and OTel Receiver Creator must not both instantiate the same receiver/resource. A feature-flagged migration must switch ownership per target and prove parity. Configuration owns proposal lifecycle and approval; existing proposal records/CLI remain only until their consumers migrate, migration-12 fixtures pass, and zero-consumer plus rollback checks permit removal.

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

Policy beyond already-approved Core/local authority and revision semantics,
precise identity mapping to OpAMP instance UID, and production Core/JetStream
provisioning interoperability remain separate open decisions where the
control-plane contract has not been approved. PR-09's Agent Operations
executor and narrow `container.restart` safety policy are implemented and
verified, but restart stays disabled by default. The source ownership choices
for persistence migration, proposals and capability catalog are tracked in the
[PR-09.5 ownership map](consolidation/02-context-ownership.md).
