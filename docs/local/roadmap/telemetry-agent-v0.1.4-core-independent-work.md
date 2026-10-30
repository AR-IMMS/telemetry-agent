# Core-Independent Delivery Roadmap — Telemetry Agent v0.1.4

**Status:** Draft for developer review  
**Basis:** Accepted Target Architecture v0.1.4 and [latest architecture gap analysis](../architecture/telemetry-agent-v0.1.4-gap-analysis.md). This roadmap covers work that can be developed and verified without AR-IMMS Core.

## 1. Milestone Overview

| Milestone | Capability | v0.1.4 | Depends on | Can proceed without Core |
|---|---|---:|---|---|
| S1 | Integration Core contracts and local persistence seam | Required | Existing SQLite and current configuration lifecycle | Yes |
| S2 | Collector capability catalog and configuration compiler | Required | S1; pinned Collector inventory | Yes |
| S3 | Reconciliation and Redis reference integration | Required | S1, S2; local desired-instance input decision | Yes |
| S4 | Resource discovery and inventory | Required | S1; connect to S3 reconciliation after S3 contracts stabilize | Yes |
| S5 | Health, diagnostics, and local status outbox | Required | S3; full integration evidence after S4/S6 | Yes |
| S6 | PostgreSQL, MongoDB, and application telemetry coverage | Required | S1–S3; capability and credential proofs | Yes |

This order establishes shared integration and compilation seams before service-specific integrations. S4 discovery-provider feasibility and S6 capability inventory can begin in parallel with S2/S3 after S1 contracts are reviewed. S5 is the final cross-cutting verification milestone.

## 2. Dependency Diagram

~~~mermaid
flowchart LR
    S1["S1 Integration Core"]
    S2["S2 Collector Compiler"]
    S3["S3 Reconciliation + Redis reference"]
    S4["S4 Resource Discovery"]
    S5["S5 Health & Diagnostics"]
    S6["S6 PostgreSQL / MongoDB / app telemetry"]
    CORE["Deferred: Core Registration/Auth,<br/>Core Sync, Core Status sender"]

    S1 --> S2 --> S3
    S1 -. "provider feasibility can start" .-> S4
    S3 -->|"wire discovered candidates"| S4
    S3 --> S6
    S3 --> S5
    S4 --> S5
    S6 --> S5
    CORE -. "requires approved Core contract" .-> S1
~~~

The Core node is intentionally disconnected from S1–S6. Core-specific adapters require approved contracts later; these milestones must not call Core, require Core credentials, or claim Core-managed configuration behavior.

## 3. Milestone Details

### S1 — Integration Core Contracts and Local Persistence Seam

**Evidence:** Gap analysis §5 Integration Context and §7 deviations; target §§3–4 and §7. Current internal/dependency.Catalog models Agent-managed host resources and is coupled to installer/inspector/receiver wiring; there is no general Integration Definition/Instance model.

**Scope & Goals**

Establish only the minimum domain model needed to represent an integration definition, a configured instance bound to a resource, required Collector capabilities, matching input/output, and configuration contribution. Keep contracts independently implementable and avoid requiring every integration to implement discovery, preflight, health, and telemetry interfaces.

**Key Components / Deliverables**

- Integration Definition, Integration Instance, and immutable component-intent value types.
- Narrow registry and contributor/matcher seams, with validation and version compatibility rules.
- Context-owned SQLite repository using the existing state.db and migration conventions; preserve existing dependency state, generation data, installation identity, enrollment state, and Gateway state.
- A reviewed local desired-instance input and ownership rule for Core-independent operation. Record source/provenance; do not decide future Core/local conflict precedence implicitly.
- Keep existing internal/dependency implementations and CLI behavior working; adapt only where a real shared seam is needed.

**Dependencies**

Existing agentstate.Repository/SQLite WAL, internal/config models, and accepted target ownership rules. Before S3 can exercise a local integration, approve how a local operator creates or edits an instance while Core integration is absent. This decision does not settle future Core precedence.

**Acceptance Criteria**

- Pure tests validate definitions, instances, capability references, duplicate IDs, and invalid contributions.
- SQLite tests prove instance updates preserve unrelated M1/M2 state and survive restart.
- A test contributor can be registered and invoked without changing Lifecycle Host or Collector process code.
- Existing dependency lifecycle and configuration tests retain their behavior.

### S2 — Collector Capability Catalog and Configuration Compiler

**Evidence:** Gap analysis §5 Collector Runtime Control and Open Questions; target §§3–4, §7, and §11. The Agent pins Collector 0.160.0 and has native validation/rendering, but no explicit capability catalog or compiler for integration intents. Exact packaged factories remain unverified.

**Scope & Goals**

Compile baseline configuration plus integration component intents deterministically, then validate with the pinned Collector’s native validator. Reuse internal/config, bootstrap validation, current activation, supervisor, and LKG recovery. Keep integration policy outside the compiler and process control outside integration contributors.

**Key Components / Deliverables**

- Explicit catalog of factories available in the Agent’s pinned Collector distribution.
- Deterministic compiler from immutable intents to complete Collector configuration; reject unknown capabilities before activation.
- Stable component naming/topology rules that allow multiple instances without assuming one pipeline per integration.
- Integration with current native validation and configuration activation/recovery paths.
- Windows/Linux component-factory and configuration smoke checks against the exact shipped artifacts.

**Dependencies**

S1 component-intent contract; current layered configuration renderer and activation path. The catalog cannot be finalized until the exact packaged Collector build/factory inventory is checked.

**Acceptance Criteria**

- Same inputs produce the same compiled config; unsupported capabilities fail before activation.
- Existing baseline, host metrics, and managed dependency receivers remain present and equivalent.
- Invalid config does not replace active/LKG config; unchanged effective config does not trigger an unnecessary restart.
- Native validation succeeds for the selected catalog on Windows and Linux pinned artifacts.

### S3 — Reconciliation and Redis Reference Integration

**Evidence:** Gap analysis §5 Reconciliation Context and Integration Context, §6 flows; target §§5–6 and §9. Existing configurationCoordinator and runtimeWatcher apply local generations, but do not reconcile desired, discovered, integration-instance, and applied snapshots. Redis is absent from current integration definitions.

**Scope & Goals**

Add the smallest revision-bound reconciliation controller over durable local desired instances and current applied state. Prove the shared path with one built-in Redis definition/contributor only after S1/S2 contracts exist. A Redis instance must use the standard compiler and activation path; it must not add Redis cases to central orchestration.

**Key Components / Deliverables**

- Immutable reconciliation input snapshot and plan carrying source revisions; idempotent plan computation and stale-plan detection.
- One serialized application path reusing SQLite, ApplyDesiredConfiguration, generation-bound Collector readiness, and existing rollback/LKG mechanisms.
- Redis definition, matcher/config contributor, and one-instance configuration using the pinned Redis receiver if present.
- Local operator path for creating/updating the Redis instance, subject to S1 ownership review.
- Test fixture with Redis and a local OTLP capture sink; do not require Core or a production Gateway.

**Dependencies**

S1 models/repository and approved local desired-state source; S2 capability catalog/compiler; current configuration activation and supervision. Secure local credential handoff to Collector is a proof gate: reuse credentialstore only if secret values can be passed without storing them in SQLite, rendered config, or logs. Do not treat Core enrollment credentials as an integration credential source.

**Acceptance Criteria**

- Repeating reconcile with unchanged inputs is a no-op; stale plans cannot mark newer inputs applied.
- Concurrent local updates preserve unrelated dependency ownership and configuration generations.
- Redis metrics reach the local OTLP capture sink after activation and after Agent restart.
- Invalid Redis configuration preserves the prior active config; Redis unavailability does not stop host metrics where the shared Collector topology permits.
- Health evidence is not used as an automatic rollback command.
- Credentialed Redis behavior is not claimed until the secret handoff passes its security tests.

### S4 — Resource Discovery and Inventory

**Evidence:** Gap analysis §5 Resource Discovery & Inventory and §8 dependency diagram; target §§3–4, §5, §7 and §9. Current platform identity and inspection of known dependencies are not a normalized inventory or provider registry.

**Scope & Goals**

Add a small provider registry and persisted normalized observations with stable resource identity, provider/provenance, observation time, and freshness/expiry. Discovery produces candidates for matching; it does not silently enable integrations or execute Collector lifecycle operations.

**Key Components / Deliverables**

- Discovery provider and inventory repository contracts, with short SQLite transactions in the existing database.
- One bounded Windows/Linux provider selected after a source/API feasibility spike; expand providers only when acceptance needs them.
- Candidate matching against S1 definitions and an explicit operator approval/configuration step before an instance becomes desired.
- Provider failure and stale-observation policy that preserves valid last observations until explicit expiry.

**Dependencies**

S1 resource-binding and matcher contracts; SQLite persistence. Provider selection is an open technical decision: the target does not identify which Windows/Linux source can provide usable service endpoint facts, and current code has no general discovery provider. The provider spike must settle this before implementation scope is fixed. Reconciler integration depends on S3.

**Acceptance Criteria**

- Provider observations are normalized and persisted with provenance and freshness.
- Repeated discovery is idempotent; transient provider failure does not erase a still-valid observation.
- Candidate discovery alone does not enable or activate an integration.
- Approved candidates become ordinary desired instances and use the same S3 reconciliation/compiler path.
- Provider tests run on Windows and Linux or explicitly isolate unsupported platform coverage.

### S5 — Health, Diagnostics, and Local Status Outbox

**Evidence:** Gap analysis §5 Health & Diagnostics and Infrastructure & Persistence, §6; target §§3–5 and §8–9. agenthealth currently provides an in-memory snapshot and local HTTP status, but no independent probe registry, scoped HealthChanged, or durable latest-status outbox.

**Scope & Goals**

Turn health into a read-only observer of Collector runtime, integration/source signals, and inventory freshness. Persist the latest status/evidence and a bounded latest-status outbox in the existing SQLite database. Publish scoped HealthChanged notifications as advisory events; do not add a general event-delivery framework or automatic rollback.

**Key Components / Deliverables**

- Narrow health probe/evidence contracts and evaluator; scope evidence by Agent, Collector, integration instance, and configuration generation where applicable.
- Durable latest snapshot/outbox repository with bounded retention and restart recovery.
- HealthChanged publisher for local consumers and current status API integration.
- Keep Core status transport/acknowledgement out of this milestone.

**Dependencies**

S3 runtime generation and Redis evidence; S4 discovery freshness for inventory health; existing agenthealth reporter/HTTP endpoint and SQLite repository conventions.

**Acceptance Criteria**

- Health transitions include scope, reason, observed time, and related generation/instance.
- Health evidence is read-only to activation; an event alone never changes desired state or triggers rollback.
- Latest status survives restart and outbox storage remains bounded.
- Redis failure can be reported without misclassifying unrelated host collection as failed where Collector topology allows.
- Core status delivery remains absent and clearly separate from local outbox persistence.

### S6 — PostgreSQL, MongoDB, and Application Telemetry Coverage

**Evidence:** Gap analysis §5 OpenTelemetry Data Plane and §9 P1; target §§7 and 9–10. Target shipping scope includes Redis, PostgreSQL, MongoDB, application metrics/logs; only existing OTel ingress and host/managed-exporter paths are present.

**Scope & Goals**

Add the remaining required v0.1.4 service/application telemetry through S1–S3 contracts and pinned OTel capabilities. Use OTel receivers/exporters first; do not add custom Go collection where a supported Collector capability exists.

**Key Components / Deliverables**

- PostgreSQL and MongoDB definitions/contributors using existing cataloged Collector factories.
- Application metrics/logs through verified OTel capabilities and the smallest supported local configuration path.
- Reusable contract/config tests for each definition plus service-specific config tests.
- Credential delivery and diagnostics reuse the approved S3 secret boundary; no secrets in SQLite, config artifacts, or logs.
- Fault-injection coverage for one integration failing while unrelated host metrics continue where pipelines permit.

**Dependencies**

S1–S3 complete; exact Collector capability inventory from S2; approved local desired-instance path; validated secret handoff; service fixtures or test containers that do not require Core. S6 can run in parallel with S4/S5 after S3 if its definitions do not depend on discovery evidence or Health APIs.

**Acceptance Criteria**

- Each required integration compiles using the common compiler and activates through the common reconciler.
- Metrics/logs from test services reach a local OTLP capture sink on supported Windows/Linux configurations.
- Restart restores the committed config; invalid config preserves LKG.
- Service outage and invalid credentials produce scoped evidence and do not unnecessarily interrupt unrelated telemetry.
- No integration adds Core HTTP, registration, authentication, or Core-specific orchestration.

## 4. Core-Dependent Work — Deferred

These capabilities require Core-owned contracts or a Core test environment and are not part of S1–S6:

- Registration binding, Agent authentication lifecycle, credential revocation, and authorization against installation/registration.
- Authenticated Core desired-state snapshot polling and Core/local precedence for overlapping fields.
- Core status transmission, acknowledgement, retry contract, and server-side reporting retention. S5 prepares only a bounded local latest-status outbox.
- Lost enrollment-response credential recovery and real enrollment end-to-end validation.

The Core-independent work must leave explicit ports and provenance where needed, but must not invent routes, payloads, authentication behavior, or a conflict-resolution policy.

## 5. Required vs Optional

| Classification | Capabilities |
|---|---|
| Required for v0.1.4 | Minimal integration definitions/instances and contribution seam; Collector capability catalog/compiler; reconciliation and safe activation; basic discovery/inventory; Redis, PostgreSQL, MongoDB and application telemetry; scoped health/diagnostics; Windows/Linux verification and restart/failure recovery. |
| Core-dependent and deferred | Registration/Auth integration, Core configuration sync, Core status sender/ack, lost-response credential recovery, and any Core/local conflict rule not yet approved. |
| Optional / future | Dynamic Go plugins, marketplace/package loading, fully autonomous discovery activation, additional providers beyond the first necessary set, distributed/multi-Collector reconciliation, guaranteed zero-loss/hot-swap, and automatic health-triggered rollback without proven causality. |

## 6. Major Risks and Review Gates

- **Ownership gate before S3:** approve the interim local operator source for integration desired state; the accepted target leaves overlapping Core/local field precedence open.
- **Collector gate before S2/S6:** verify the exact 0.160.0 shipped factory inventory and Windows/Linux artifacts. A version pin alone is not evidence that a receiver is available in the packaged binary.
- **Credential gate before credentialed service coverage:** prove how local secrets reach Collector runtime without plaintext persistence or logging. Do not assume current CredentialStore has a safe Collector handoff.
- **Discovery gate before S4 scope lock:** select a Windows/Linux provider that can supply actionable resource facts; do not equate dependency status inspection with general service discovery.
- **Blast-radius gate in S3/S6:** verify integration failure behavior with the shared Collector process/pipelines; do not promise per-integration process isolation.
- Keep each non-test Go file at or below 450 LOC; split only when a cohesive responsibility requires it, and preserve current dependency CLI, SQLite state, activation, supervision, and recovery behavior.

**Source evidence:** [Target Architecture v0.1.4](../architecture/target-architecture-v0.1.4.md), especially §§3–11; [Architecture Gap Analysis](../architecture/telemetry-agent-v0.1.4-gap-analysis.md), especially §§5–10; current implementation symbols cited in the gap analysis include dependency.Catalog, collectorConfigurationActivator, configurationCoordinator, runtimeWatcher, agenthealth.Reporter, agentstate.Repository, and the pinned bootstrap.collectorVersion.


