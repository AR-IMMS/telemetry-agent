# Telemetry Agent v0.1.4 Architecture Audit & Gap Analysis

**Assessment date:** 31 October 2026  
**Audited checkout:** epic/m2-local-installation-identity, HEAD 2773145ae803f688608677e1256b78728f889b1f  
**Scope:** Read-only assessment of the Agent repository against the accepted Target Architecture v0.1.4. No implementation changes are included.

## 1. Executive Summary

The Agent has a working local control-plane foundation: CLI and service lifecycle, per-installation identity persistence, an Agent-side enrollment workflow, SQLite WAL operational state, layered Collector configuration, local dependency management, Collector process supervision, generation-bound readiness, and last-known-good recovery.

It does **not** yet implement the full v0.1.4 control loop. The main missing links are authenticated Core configuration polling and status reporting, general resource inventory/discovery, Integration Definition/Instance contracts, a continuous reconciliation controller over desired/discovered/applied snapshots, the capability catalog and compiler for arbitrary integration intents, and independent health evidence/events.

No bounded context is fully complete against its entire v0.1.4 responsibility. Several important scoped capabilities are implemented and have unit or persistence tests. Real Core enrollment and Core configuration compatibility, shipped Collector smoke tests, Gateway telemetry receipt, and native Windows/Linux runtime behavior remain unverified in this audit environment.

| Final category | Result |
|---|---|
| Fully implemented scoped capabilities | SQLite-backed installation identity; Agent-side durable enrollment state machine; local layered OTel rendering and native validation; current Collector supervision/readiness/recovery; local dependency lifecycle and ownership checks. |
| Partially implemented contexts | Lifecycle Host; Identity & Connectivity; Configuration & Desired State; Resource Discovery; Integration; Reconciliation; Collector Runtime Control; Health & Diagnostics; Infrastructure & Persistence; OTel Data Plane. |
| Not implemented target capabilities | Authenticated Core polling/reporting; durable status outbox; general inventory/provider registry; general Integration Definition/Instance framework; continuous reconciliation queue/controller and immutable plans; HealthChanged events; component capability catalog. |
| Unverified capabilities | Core contract/authorization/lost-response recovery; cloned-VM identity behavior; native Windows/Linux end-to-end installation and Collector smoke tests; Gateway delivery; exact packaged component factory inventory. |
| Critical blockers | Core-side authenticated Agent management contract and test environment; missing Agent polling/reporting path; missing general integration/discovery/reconciliation chain required for M3 coverage. |
| v0.1.4 readiness | **Not ready.** Suitable as a local host-telemetry and managed-dependency Agent foundation; the target’s Core-managed, discovery-driven integration behavior is incomplete. |

## 2. Source of Truth & Evidence

### Authority and conflicts

1. **Approved target:** docs/local/architecture/target-architecture-v0.1.4.md, version 0.1.4, accepted 26 October 2026. Its target sections and bounded-context requirements are authoritative.
2. **Corroborating shared overview:** docs/shared/architecture/telemetry-agent-overview.md, current-code status dated 30 October 2026. It agrees with source that enrollment is Agent-side and Core sync/reporting are incomplete; it is not the target authority.
3. **Historical/superseded:** docs/local/architecture/target-agent-architecture.md explicitly says it is superseded. The accepted target’s section 12 is a repository gap snapshot dated 26 October and predates later M2 work. Its older JSON-state and missing-installation-identity claims are stale against HEAD; this audit rechecked source and uses neither as current facts.
4. docs/local/architecture/m2-core-gateway-endpoint-sync-design.md is a proposed Agent-side design. It explicitly records that the Core endpoints/schemas and credential behavior are not approved/verified. It is not evidence of a running Core integration.
5. No AR-IMMS Core backend source or approved OpenAPI contract is present in this checkout or the shared-docs submodule. Core authorization, endpoint compatibility, revocation, and lost-response credential recovery therefore remain unknown.

### Classification and evidence language

| Label | Meaning |
|---|---|
| **Verified Fact** | Present in the audited source or directly exercised by a test run below. A source fact alone does not prove end-to-end runtime behavior. |
| **Approved Target Requirement** | Explicitly stated by the accepted v0.1.4 target. |
| **Inference** | Consequence derived from the verified source and target; not itself a separately implemented/approved contract. |
| **Unknown / Unverified** | The repository or available execution environment cannot establish the behavior. |

### Verification performed

- Inspected target, shared overview, M2 designs, roadmap, Go source, composition/wiring, configuration, SQLite adapters, platform adapters, and relevant tests.
- go test ./internal/coreconfig ./internal/enrollment ./internal/coreapi ./internal/identity ./internal/agenthealth ./internal/supervisor ./internal/agentlifecycle -count=1: **failed as an environment-limited run**. coreconfig, enrollment, coreapi, and identity passed. agenthealth tests could not bind local sockets; supervisor readiness callback test could not complete against its local test listener; agentlifecycle persistence/config tests failed on sandbox-denied atomic renames under the OS temp directory.
- go test ./... -count=1: **not completed**; the run emitted failures and was interrupted after no further output. Failures included the same Windows sandbox rename/socket restrictions and help tests whose expected output omits the current enroll command. Do not treat this as a complete suite result.
- No Collector binary was available on PATH during the audit. No native Windows/Linux service run, packaged Collector smoke test, real Core call, or Gateway receipt test was performed.
- git status could not fully inspect the repository because Git was denied access to the docs/shared submodule metadata. Repository-wide cleanliness is therefore unverified.

## 3. Approved Target Architecture Diagram

The diagram below reproduces the accepted target’s contexts, named elements, and communication boundaries. It is a target view, not a claim about implementation.

~~~mermaid
flowchart TB
    subgraph EXTERNAL["External Systems"]
        CORE["AR-IMMS Core API"]
        GATEWAY["OTLP Gateway"]
        SYSTEM["Windows / Linux Host or VM"]
        SERVICES["Redis / PostgreSQL / MongoDB / Applications"]
    end

    subgraph AGENT["AR-IMMS Telemetry Agent — Control Plane"]
        subgraph HOST["Lifecycle Host"]
            ENTRY["CLI / OS Service / Diagnostics API"]
            BOOT["Bootstrap & Shutdown"]
            SUP["Process Supervision"]
        end
        subgraph ID["Identity & Connectivity Context"]
            REG["Registration & Installation Identity"]
            AUTH["Credential / Authentication Session"]
            SYNC["Core Configuration Sync"]
            OUTBOX["Status Reporting / Outbox"]
        end
        subgraph CFG["Configuration Context"]
            BASE["Baseline Configuration"]
            DESIRED["Desired State Store"]
            POLICY["Configuration Precedence"]
            ACT["Activation Coordinator"]
            REVISIONS["Applied Revision / Last-Known-Good"]
        end
        subgraph RES["Resource Inventory Context"]
            DP["Discovery Provider Registry"]
            DISC["Host / VM / Service / Container Discovery"]
            INV["Resource Inventory"]
        end
        subgraph INT["Integration Context"]
            REGISTRY["Integration Registry"]
            MATCH["Integration Matcher"]
            INST["Integration Instances"]
            PREFLIGHT["Preflight Validation"]
        end
        subgraph REC["Reconciliation Context"]
            QUEUE["Reconcile Queue"]
            CONTROLLER["Reconciliation Controller"]
            PLAN["Immutable Configuration Plan"]
        end
        subgraph RT["Collector Runtime Control Context"]
            CAT["Collector Capability Catalog"]
            COMP["Configuration Compiler"]
            VALID["Collector Config Validator"]
            LCM["Collector Lifecycle Adapter"]
        end
        subgraph HEALTH["Health Context"]
            PROBES["Health Probes"]
            EVAL["Health Evaluator"]
            SNAP["Health Snapshot / Evidence Reader"]
            EVENT["HealthChanged Publisher"]
        end
        subgraph INFRA["Infrastructure"]
            BUS(("Internal Event Dispatcher"))
            DB[("SQLite state.db — WAL")]
            SECRETS["Secure Credential Storage"]
            OSAD["OS / Filesystem / Network Adapters"]
        end
    end

    subgraph OTEL["OpenTelemetry Collector — Data Plane"]
        RECEIVERS["Receivers"]
        PROCESSORS["Processors"]
        EXPORTERS["Exporters"]
        STORAGE[("Optional Persistent Queue")]
        RECEIVERS --> PROCESSORS --> EXPORTERS
        EXPORTERS -.-> STORAGE
    end

    ENTRY --> BOOT --> SUP
    SUP -.->|"Lifecycle only"| ID
    SUP -.->|"Lifecycle only"| CFG
    SUP -.->|"Lifecycle only"| RES
    SUP -.->|"Lifecycle only"| INT
    SUP -.->|"Lifecycle only"| REC
    SUP -.->|"Lifecycle only"| HEALTH
    REG --> AUTH
    SYNC --> DESIRED
    BASE --> POLICY
    POLICY --> DESIRED
    DP --> DISC --> INV
    INV --> MATCH
    REGISTRY --> MATCH
    MATCH --> INST
    INST --> PREFLIGHT
    DESIRED --> QUEUE
    INV --> QUEUE
    INST --> QUEUE
    QUEUE --> CONTROLLER --> PLAN
    PLAN --> COMP
    CAT --> COMP
    COMP --> VALID --> ACT
    ACT --> LCM
    ACT --> REVISIONS
    LCM -->|"Start / Restart / Apply"| OTEL
    OTEL -.->|"Runtime telemetry"| PROBES
    PROBES --> EVAL --> SNAP
    EVAL --> EVENT --> BUS
    BUS -.->|"Wake-up signal"| QUEUE
    SNAP -.->|"Read-only evidence"| ACT
    ID --> DB
    CFG --> DB
    RES --> DB
    INT --> DB
    REC --> DB
    HEALTH --> DB
    AUTH --> SECRETS
    DISC --> OSAD
    REG <--> CORE
    SYNC <--> CORE
    OUTBOX --> CORE
    DISC --> SYSTEM
    RECEIVERS --> SERVICES
    EXPORTERS --> GATEWAY
~~~


## 4. Current Implemented Architecture Diagram

Derived from code and wiring at the audited HEAD. PARTIAL and MISSING mark target responsibilities that are incomplete or absent; the diagram does not imply that same-named packages implement full target contexts.

~~~mermaid
flowchart LR
    subgraph EXT["External"]
        CORE["AR-IMMS Core"]
        GW["OTLP Gateway"]
        HOST["Windows / Linux host"]
        DEPS["Windows Exporter / Node Exporter / LibreHardwareMonitor"]
    end

    subgraph CP["Agent process — current control plane"]
        CLI["agentctl CLI / install / service"]
        COMPOSE["defaultDependencies composition"]
        ID["internal/identity<br/>machine + installation UUID"]
        ENROLL["internal/enrollment<br/>local durable attempt policy"]
        API["internal/coreapi<br/>HTTPS enrollment client"]
        CREDS["internal/credentialstore<br/>protected local files"]
        DB[("internal/agentstate<br/>SQLite WAL; state + identity + enrollment + Gateway runtime")]
        STATE["Local dependency state<br/>desired / activated / applied generations"]
        CFG["internal/config<br/>layered render + validation"]
        DEP["internal/dependency<br/>built-in local resource catalog"]
        LC["internal/agentlifecycle<br/>configuration coordinator + runtime watcher"]
        SUP["internal/supervisor<br/>Collector child + readiness"]
        HEALTH["internal/agenthealth<br/>in-memory snapshot + local HTTP"]
        SNAP["internal/coreconfig<br/>Gateway snapshot policy<br/>(local acceptance seam only)"]
        BOOT["internal/bootstrap<br/>pinned Collector install + validate + config recovery"]
        NO_SYNC["Core polling / authenticated desired sync<br/>MISSING"]
        NO_DISC["Resource inventory / provider registry<br/>MISSING"]
        NO_INT["General Integration Definition / Instance<br/>MISSING"]
        NO_REC["Continuous desired + discovered reconciler<br/>MISSING"]
        NO_OUT["Durable status outbox / Core reporting<br/>MISSING"]
        NO_EVENTS["HealthChanged / event dispatcher<br/>MISSING"]
    end

    subgraph DP["OpenTelemetry Collector — separate data plane"]
        OTLP["OTLP receiver"]
        HM["hostmetrics receiver"]
        PROM["Prometheus receivers"]
        PIPE["processors / metrics, logs, traces pipelines"]
        EXPORT["OTLP exporter"]
        OTLP --> PIPE
        HM --> PIPE
        PROM --> PIPE
        PIPE --> EXPORT
    end

    CLI --> COMPOSE
    COMPOSE --> ID
    CLI --> ENROLL
    ENROLL --> API
    ENROLL --> CREDS
    ENROLL --> DB
    ID --> DB
    CLI --> STATE
    DEP --> STATE
    STATE --> CFG
    CFG --> BOOT
    LC --> STATE
    LC --> CFG
    LC --> SUP
    LC --> DB
    SUP --> DP
    HEALTH -.-> LC
    HEALTH --> CLI
    SNAP --> DB
    SNAP -.-> LC
    BOOT --> SUP
    HOST --> ID
    DEPS --> PROM
    EXPORT --> GW
    ENROLL --> CORE
    CORE -.-> NO_SYNC
    NO_SYNC -.-> DB
    HOST -.-> NO_DISC
    NO_DISC -.-> NO_REC
    NO_INT -.-> NO_REC
    NO_REC -.-> CFG
    HEALTH -.-> NO_EVENTS
    NO_OUT -.-> CORE

    classDef implemented fill:#dcfce7,stroke:#15803d,color:#111827
    classDef partial fill:#fef3c7,stroke:#b45309,color:#111827
    classDef missing fill:#fee2e2,stroke:#b91c1c,color:#111827
    classDef dataplane fill:#e0e7ff,stroke:#4338ca,color:#111827
    class CLI,ID,ENROLL,API,CREDS,DB,CFG,DEP,LC,SUP,BOOT implemented
    class COMPOSE,STATE,HEALTH,SNAP partial
    class NO_SYNC,NO_DISC,NO_INT,NO_REC,NO_OUT,NO_EVENTS missing
    class OTLP,HM,PROM,PIPE,EXPORT dataplane
~~~

**Current flow notes:** agentctl enroll invokes the Agent enrollment workflow directly; there is no runtime Core sync worker. The Gateway snapshot repository accepts a validated value only when called and documents that its caller must authenticate; no authenticated caller is wired. Local dependency state changes trigger config activation through CLI/lifecycle operations and a runtime watcher; there is no queue/controller continuously reconciling discovered inventory.

## 5. Target vs Current Comparison Matrix

Evidence paths and line ranges refer to the audited Agent checkout unless marked target/shared. In this matrix, Target Responsibility is the Approved Target Requirement, Current Implementation is the Verified Fact, and Gap is an inference unless marked unknown. Test names are cited as corroboration, not as a claim of end-to-end success.

| Target Component | Target Responsibility | Current Implementation | Status | Evidence | Gap |
|---|---|---|---|---|---|
| Lifecycle Host | Composition root; CLI/OS service startup and shutdown; Collector process supervision; local diagnostics; no integration policy hub. | agentctl dispatches commands, composes concrete dependencies, serves local status and runs the Collector runtime. The composition also declares the named dependency catalog. | PARTIAL | cmd/agentctl/main.go:13-25,44-100; composition.go:36-69,139-210,276-360; collector_command.go:73-164; internal/agentinstallation/ | Operational lifecycle exists. The composition/command layer remains a central wiring point for named integrations, and service/runtime behavior has not been verified natively on both OSes here. |
| Identity & Connectivity | Per-installation identity, registration binding, credentials, authenticated Core sync/connectivity, bounded outbound reports/outbox. | UUIDv4 installation identity is initialized through SQLite; enrollment has a durable Agent-side request/retry flow and protected credential adapter. No Core polling, authenticated configuration client, connectivity status, or reporting outbox is wired. | PARTIAL | internal/identity/installation_identity.go:10-50; internal/agentstate/sqlite_identity.go:13-65; internal/enrollment/service.go:82-140,214-313; internal/coreapi/enrollment_client.go:26-103; cmd/agentctl/enrollment_command.go:92-128; docs/shared/architecture/telemetry-agent-overview.md:43-46,58-76 | Real Core enrollment/auth behavior and lost-response credential recovery are unverified; authenticated Core configuration and reporting are absent. |
| Configuration & Desired State | Baseline, durable desired revisions, precedence/ownership, activation, verification, applied/LKG metadata and rollback policy. | SQLite state stores dependency desired/activated/applied generations and pending activation; Core Gateway policy stores accepted/activated/applied revision mappings, ownership and LKG endpoint. Renderer validates and lifecycle can apply/recover configuration. | PARTIAL | internal/agentstate/state.go; internal/agentstate/sqlite_store.go:41-109; internal/coreconfig/gateway.go:23-87; internal/agentstate/sqlite_core_gateway.go:28-120,169-302; internal/agentlifecycle/configuration.go:28-129; collector_configuration.go:45-175 | No general precedence model or Core ingress. Desired payload is limited to local dependency state plus one Gateway endpoint; broader config revisions/ownership are not modeled. |
| Resource Discovery & Inventory | Provider registry and normalized host/VM/service/container observations with identity, provenance, freshness and expiry. | Platform identity and inspection of known, explicitly managed local dependencies exist; they do not form a general inventory or discovery provider registry. | PARTIAL | internal/identity/platform*.go; internal/dependency/*/inspect.go; internal/dependency/catalog.go:82-99; no inventory/provider package or symbol found under internal/ | No persisted discovered state, provider observations, freshness/expiry rules, or discovery-triggered reconciliation. |
| Integration Context | Versioned definitions, instances, registry/matching, capability requirements, preflight and config contributions; no runtime/process control. | dependency.Catalog maps a fixed set of managed host resources to installers, inspectors, receiver names and teardown. It is useful local-resource management, not the target definition/instance model. | PARTIAL | internal/dependency/types.go:11-63; catalog.go:13-133; cmd/agentctl/composition.go:276-360; internal/agentlifecycle/collector_configuration.go:51-65 | No Redis/PostgreSQL/MongoDB/application definitions or instances, matcher, independent extension contracts, preflight contributions, or general integration configuration. |
| Reconciliation Context | Queue advisory triggers; read consistent desired/discovered/applied snapshots; compute immutable, revision-bound idempotent plans and retry safely. | configurationCoordinator snapshots local state, checks generation staleness, serializes config activation, and commits an activated generation; runtimeWatcher polls state and starts the activated config. | PARTIAL | internal/agentlifecycle/configuration.go:13-129; internal/agentlifecycle/plan.go; runtime.go:48-272,273-331; internal/agentlifecycle/configuration_test.go:209 | This is a local configuration lifecycle, not a continuous reconciler over desired + discovered + integration-instance snapshots. No reconcile queue, retry scheduling or plan source revisions beyond local generations. |
| Collector Runtime Control | Explicit capability catalog, deterministic compiler, native validation, lifecycle adapter and runtime status. | Config layering and validation invoke the pinned Collector binary; lifecycle snapshots config/endpoint, supervises process and commits applied state after generation-bound readiness. No capability catalog or general intent compiler exists. | PARTIAL | internal/bootstrap/types.go:12-95; internal/config/renderer.go:14-100; internal/agentlifecycle/collector_configuration.go:16-175; internal/agentlifecycle/runtime.go:273-331; internal/supervisor/readiness.go:18-121 | Existing runtime behavior is substantial, but component availability is not represented by an explicit catalog and integration config is hard-wired to dependency receiver names. |
| Health & Diagnostics | Independent probes/evidence, scoped evaluation and snapshots, HealthChanged, read-only contracts; no configuration mutation or rollback authority. | In-memory Reporter evaluates Collector state/generation/pending teardown; local /v1/status API exposes snapshot; dependency inspections also yield health values. No event publisher, persisted evidence history or independent probe registry. | PARTIAL | internal/agenthealth/snapshot.go:6-71; reporter.go:5-67; http.go:9-54; internal/agentlifecycle/runtime.go:333-369; internal/dependency/types.go:84-143 | Health is a local status summary rather than the target observer/evidence context. No HealthChanged event or Core status reporting. |
| Infrastructure & Persistence | One SQLite WAL operational DB, short transactions, context-owned repositories, OS/network/filesystem adapters, protected credential store and bounded outbox. | SQLite WAL is the shared operational store for state, identity, Gateway revisions and enrollment attempt metadata; credential values are stored separately. SQLite updates use short transactions and compatibility import. | PARTIAL | internal/agentstate/repository.go:15-48; sqlite_store.go:19-140; sqlite_schema.go:13-50; sqlite_core_gateway_schema.go; sqlite_enrollment_schema.go; internal/credentialstore/store.go:16-109 | No durable latest-status outbox or bounded reporting delivery. Credential file protection differs by OS (Windows DPAPI; Linux file permissions, not encryption); deployment/security validation is not shown. |
| OpenTelemetry Data Plane | Separate Collector with configured receiver/processor/exporter/connector/extension factories, signal processing, retry/queue and OTLP output. | Separate otelcol-contrib process pinned to 0.160.0; base config includes OTLP receiver, health extension, processors and OTLP exporter; OS overlays configure hostmetrics and Prometheus scrapes. | PARTIAL | internal/bootstrap/types.go:12; configs/base/otel.yaml; configs/os/windows/otel.yaml; configs/os/linux/otel.yaml; internal/supervisor/supervisor.go:34-100; docs/local/architecture/metrics-spine.md | Existing host/dependency data path is configured, but component inventory, requested service/application coverage, persistent queue behavior and actual Gateway receipt were not verified. |
| Internal Coordination | Durable state is authoritative; internal events are advisory wakeups for reconciliation. | Lifecycle runtime uses polling; no domain event dispatcher or reconcile queue was found. | NOT IMPLEMENTED | internal/agentlifecycle/runtime.go:48-120; repository-wide search found no event dispatcher/HealthChanged publisher | Add only when consumers require it; current polling does not satisfy the target event-driven wakeup contract. |

**Enrollment nuance:** Agent code persists attempt identity/payload before sending, uses bounded retries/resume, validates response identity and same-origin HTTPS, and keeps secret values outside SQLite. Tests use fakes/HTTP test servers. These establish an Agent-side implementation, not successful authorization, revocation behavior, or lost-response credential recovery in the real Core. See internal/enrollment/service_test.go, internal/coreapi/enrollment_client_test.go, and the shared overview lines 58–76.


## 6. Data Flow Coverage

| Flow | Status | Current evidence | First missing link / limit |
|---|---|---|---|
| Installation identity and enrollment | PARTIAL | SQLite UUID initialization is wired before runtime; agentctl enroll composes repository, credential store, and HTTPS client. | Real Core contract/authz and end-to-end response recovery are unverified. Identity cloning/reinstall lifecycle tests are absent. |
| Core authentication and connectivity | NOT IMPLEMENTED | Enrollment stores the returned credential reference/value separately, but no authenticated sync/report client consumes it. | No registration-bound authentication session, polling worker, connectivity state, or Core sync adapter. |
| Core desired configuration synchronization | PARTIAL | Local Gateway snapshot validation/revision/ownership persistence and runtime mapping exist. | No authenticated Core snapshot fetch, Core endpoint contract, polling/retry loop, or ingress binding. |
| Desired-state reconciliation | PARTIAL | Local state changes are serialized and activated by lifecycle operations; runtime watcher observes activated generation. | No reconcile controller that reads desired, discovered, integration, and applied snapshots. |
| Integration configuration and activation | PARTIAL | Existing managed dependencies map to Prometheus receivers and trigger full config render/validate/activate. | No general Integration Definition/Instance config contribution; Redis/PostgreSQL/MongoDB are not delivered. |
| Resource discovery | PARTIAL | OS identity and known dependency inspectors provide local facts. | No general discovery provider registry or persisted normalized resource inventory. |
| Collector supervision and recovery | PARTIAL | Supervisor starts/stops child, waits for exact health response body while child lives; activation journal and LKG recovery are implemented in source/tests. | No native service/Collector smoke test in this environment; supervisor/lifecycle tests were sandbox-blocked. |
| Telemetry delivery | PARTIAL | Configured OTLP exporter sends to OTEL_GATEWAY_ENDPOINT; Collector owns pipeline processing. | Config presence is not receipt. No live Gateway end-to-end result or delivery evidence; exporter uses tls.insecure: true in base config. |
| Health and status reporting | PARTIAL | Local status endpoint returns evaluated transient snapshot. | No scoped HealthChanged, durable latest-status outbox, Core delivery, or Gateway delivery health evidence. |
| Restart and failure recovery | PARTIAL | SQLite state and activation recovery preserve local state and LKG; enrollment attempts can resume. | Core outage/reconnect polling cannot be exercised because sync is absent; crash-window and native OS service tests remain unverified. |

## 7. Architecture Deviations

Only source-supported conflicts with explicit target boundaries are listed. Missing target work is otherwise classified as a gap, not a violation.

| Deviation | Target Requirement | Current Behavior | Impact | Evidence |
|---|---|---|---|---|
| Integration receiver selection is coupled to the local dependency catalog and lifecycle compiler. | Integration context emits component intent; compiler consumes immutable intents/capability catalog; adding an integration using existing capability should not require Reconciler/core orchestration edits. | collectorConfigurationActivator asks dependency.Catalog for enabled receiver strings and injects them into a hard-coded receiver list. The catalog is populated with named OS dependency implementations in the CLI composition root. | A new service integration currently needs edits to central dependency/composition and Collector compilation code; no extension contribution seam is exercised. | internal/agentlifecycle/collector_configuration.go:16-65; cmd/agentctl/composition.go:276-360; target docs/local/architecture/target-architecture-v0.1.4.md:195-215,247-271. |
| Lifecycle composition contains product-specific integration registration. | Lifecycle Host constructs adapters but must not contain integration-specific logic or become a central business orchestration hub. | defaultDependencyCatalog wires Windows Exporter, Node Exporter, and LibreHardwareMonitor-specific installers, inspectors, receiver IDs and teardown behavior in cmd/agentctl. | The composition root is not yet a neutral host for independently added integrations; integration registration changes the host source. | cmd/agentctl/composition.go:276-360; target docs/local/architecture/target-architecture-v0.1.4.md:191,206. |

coreconfig.GatewayRepository.AcceptGatewaySnapshot explicitly requires a trusted/authenticated caller, but no such Core ingress is wired. This is an incomplete flow, **not** evidence that current remote requests bypass authentication. No REST Core sync API exists in the audited Agent runtime.

## 8. Gap Dependency Diagram

Solid dependencies follow the accepted target or observed current wiring. Dashed red links mark external blockers or capabilities absent from this repository; they do not imply an approved Core route/schema.

~~~mermaid
flowchart LR
    CORE["External Core API<br/>contract + registration auth<br/>UNVERIFIED / BLOCKING"]
    ENROLL["Agent-side enrollment<br/>implemented; Core E2E unverified"]
    AUTH["Authenticated Core client/session<br/>MISSING"]
    SYNC["Snapshot poll + desired ingress<br/>MISSING"]
    OUTBOX["Durable latest-status outbox<br/>MISSING"]
    DB["SQLite WAL + local state<br/>IMPLEMENTED"]
    SNAP["Gateway snapshot policy + revision mapping<br/>PARTIAL"]
    REC["Continuous Reconciliation Controller<br/>MISSING"]
    DISC["Discovery providers + resource inventory<br/>MISSING"]
    INT["Integration definitions / instances / contributions<br/>MISSING"]
    CAT["Collector capability catalog + intent compiler<br/>MISSING"]
    CONFIG["Layered config + native Collector validation<br/>IMPLEMENTED for current inputs"]
    ACT["Activation + generation-bound readiness + LKG recovery<br/>IMPLEMENTED for current config lifecycle"]
    HEALTH["Health probes / evidence / HealthChanged<br/>PARTIAL / MISSING events"]
    COL["Pinned Collector process<br/>PARTIAL target capability set"]
    GW["OTLP Gateway<br/>external"]
    HOST["Host + managed local exporters<br/>current sources"]
    M3["Redis / PostgreSQL / MongoDB / apps<br/>NOT IMPLEMENTED"]
    REPORT["Core status synchronization<br/>MISSING"]

    ENROLL -.->|"Requires real Core contract verification"| CORE
    CORE -.-> AUTH
    AUTH -.-> SYNC
    SYNC -.-> SNAP
    SNAP --> DB
    DB --> REC
    DISC -.-> REC
    INT -.-> REC
    REC -.-> CAT
    CAT -.-> CONFIG
    CONFIG --> ACT --> COL --> GW
    HEALTH -.-> ACT
    HEALTH -.-> OUTBOX
    OUTBOX -.-> REPORT -.-> AUTH
    HOST --> COL
    M3 -.-> DISC
    M3 -.-> INT
~~~


## 9. Prioritized Missing Work

Priorities reflect accepted target flows and their actual dependencies. Items are gaps, not a new milestone plan.

### P0 — Architectural blockers

| Gap | Dependency | Expected outcome | Evidence |
|---|---|---|---|
| No authenticated Core configuration polling and snapshot ingress; no connected authenticated Core reporting path. | Core must provide/review the Agent management API, registration-bound credential authorization, revocation behavior, schemas, revision/ack semantics, and a test environment. Agent enrollment’s real Core compatibility is also unverified. | A registered Agent can authenticate, pull complete desired snapshots, persist them before local reconciliation, report status, and remain local-first across Core outages. | Core/backend source and approved OpenAPI are absent; cmd/agentctl wires enrollment only; no polling/reporting symbols or consumer of stored Agent access credentials were found. M2 design marks API/auth contract open. |
| No general desired/discovered/applied reconciliation loop. | Stable desired-state, inventory, integration-instance and applied-state contracts; accepted ownership rules. | Idempotent, recoverable plans from durable snapshots; changes to one integration do not require central orchestration edits. | Existing configurationCoordinator and runtimeWatcher operate on local dependency generations; no inventory/reconcile queue/controller. |

### P1 — Required for v0.1.4

| Gap | Dependency | Expected outcome | Evidence |
|---|---|---|---|
| Resource inventory and discovery provider model. | Resource identity, provenance/freshness policy, platform provider contracts, integration matching policy. | Host/service observations can drive candidate instances safely without dropping resources on transient discovery failure. | No inventory/provider implementation; target scenarios require host/VM/service discovery. |
| General Integration Definition, Instance, and configuration contribution model. | Reconciliation inputs and capability/compiler contract; explicit enablement/credential policy. | Redis, PostgreSQL, MongoDB and application telemetry can be configured through stable local contracts and shared lifecycle paths. | Current dependency catalog is for Agent-installed host exporters only; no target integration instances exist. |
| Collector capability catalog and compiler for component intents. | Exact shipped Collector build/factory inventory and Windows/Linux validation. | Unsupported components are rejected before activation; valid integrations compile to native OTel config. | Collector is pinned, but compiler only assembles existing receiver names and invokes native validation; no capability catalog. |
| Independent health evidence, scoped HealthChanged, and durable latest-status outbox. | Probe/evidence scope and severity decisions; Core reporting/ack/retention contract. | Health remains observational; status survives reconnect/restart and is delivered without a general event system. | Local in-memory Reporter and HTTP status only; no event publisher or outbox table. |
| Application logs/metrics and service integration coverage; verify partial-failure isolation. | Pinned OTel component inventory, integration definitions, credentials and failure policy. | Target coverage for applications and databases while unrelated telemetry continues where pipeline dependencies allow. | Current config has OTLP logs/traces ingress and host metrics, but no Redis/PostgreSQL/MongoDB or app-specific receivers/config. |
| Cross-platform runtime proof for install, activation, restart, service shutdown and telemetry path. | Native Windows/Linux runners, packaged pinned Collector binary, Gateway/local OTLP sink. | Evidence that current and future configuration activation works on both supported OSes and reaches an ingest endpoint. | CI matrix is defined in .github/workflows/ci.yml; this audit environment blocked socket/temp-file tests and had no Collector binary. |

### P2 — Deferred / Optional in the accepted baseline

| Capability | Basis |
|---|---|
| Arbitrary dynamic Go plugin loading, integration marketplace, and remote executable code. | Explicitly deferred by target section 10. |
| Fully autonomous discovery activation, multi-Collector sharding, distributed reconciliation, guaranteed zero-loss/hot-swap, and automatic rollback without tested causality policy. | Explicitly deferred by target section 10. |
| Agent-owned telemetry spool beyond Collector retry/persistent queue. | Target says Collector persistent telemetry storage may remain separate; prove queue need before adding another. |
| Declarative package signature/trust policy. | Target identifies package loading trust as an open decision; do not assume remote loading. |

## 10. Open Questions & Verification Limitations

| Question / unknown | Why it matters | Evidence needed |
|---|---|---|
| What exact Core routes, schemas, auth credential, installation/registration binding, revocation and status acknowledgements are approved? | No authenticated polling/reporting implementation can be safely validated without the server contract. | Core-owned OpenAPI/source and integration environment; include revoked registration and wrong-binding tests. |
| How does Core recover a credential after Agent loses the successful enrollment response? | Agent tests only simulate a fake client returning a recovered result; server-side idempotent recovery is outside this repository. | Core retry semantics and real lost-response integration test. |
| Which Collector distribution artifacts/factories are actually shipped on Windows/Linux? | Version pin alone does not establish exact capabilities or config/runtime compatibility. | Build/manifest inventory and smoke test using the exact packaged binaries on both platforms. |
| Does Linux credential protection satisfy deployment policy? | Linux adapter stores secret files with restrictive ownership/mode; it does not encrypt them in this code. | Service-account threat model and approved OS credential-storage requirement. |
| Are cloned VM, reinstall, regular uninstall and identity retention semantics correct? | UUID initialization is tested locally; clone/reinstall lifecycle policy is not proven and identity may live in data removed by uninstall. | Identity lifecycle decision and Windows/Linux VM/install/uninstall tests. |
| Does Collector readiness imply signal delivery? | It does not; current activation readiness confirms the selected Collector process/config generation, not Gateway receipt. | Separate Gateway delivery evidence and policy; target explicitly distinguishes these. |
| Which full test failures reproduce outside this sandbox? | Test run had access-denied temp renames/local sockets, plus stale help expectations for enroll; suite was interrupted. | Rerun on native Windows/Linux developer/CI runners; update only stale tests after review. |
| Can integration failures remain isolated in the shared Collector topology? | Multiple receivers can share one config/process/pipeline; readiness is process/config level. | Fault-injection tests with a failing integration and unrelated host pipeline. |

### Final assessment

- **Fully implemented target capabilities (scoped):** per-installation UUID persistence and startup initialization; Agent-side durable enrollment mechanics; deterministic layered Collector rendering/native validation for current settings; local managed-dependency lifecycle; Collector supervision with generation-bound readiness and local configuration recovery.
- **Partially implemented:** all ten target bounded contexts, because current packages deliver narrower local behaviors than their complete target responsibilities.
- **Missing:** authenticated Core sync/reporting, status outbox, general discovery/inventory, Integration Definition/Instance and extension contributions, continuous reconciliation controller/queue, capability catalog, HealthChanged, and v0.1.4 service telemetry coverage.
- **Unverified:** real Core enrollment and authorization, lost-response credential recovery, Core outage/reconnect, shipped Collector factory inventory/runtime smoke, Gateway receipt, VM clone semantics, and native Windows/Linux lifecycle end-to-end.
- **Critical blockers:** missing Core-owned authenticated management contract/test service; absent Core sync/reporting adapter; absent discovery/integration/reconciliation chain.
- **Readiness:** not ready for Target Architecture v0.1.4. The current implementation is a useful local-first foundation, but target-required Core-managed and discovery-driven integrations have not been delivered.



