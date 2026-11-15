# PR-09.5 — Flexible Source Structure

**Status:** architecture-approved (2026-11-16). The user approved all five PR-09.5 planning documents, including this capability-oriented physical map and migration disposition for SQLite versions 1–14. OD-01 through OD-06 remain the approved ownership decisions. Package names and candidate destinations remain flexible; this tree is not a required scaffold.
**Evidence:** the original source ownership and transaction audit is anchored at the PR-09 integration tree `b2599b8db31bfedfd75e360423d5aa98cfbcb5dc`. PR-09.5a was merged as `79bb5a3092f55b3645d667d4f5481bde6c44700e`; PR-09.5b was merged into `chore/v0.1.4a-next` as `0f1addd4d6bc3e11ef6eb56801033e72791751fe`. The current PR-09.5c worktree is based on that integration head. Its in-progress proposal and Configuration schema moves are worktree evidence and are not yet merged. The approved ownership decisions are recorded in [02-context-ownership](02-context-ownership.md).

**Evidence boundary:** baseline source paths are from the immutable PR-09 audit; merged destinations are checked against PR-09.5b integration at `0f1addd`, and active moves are checked against the PR-09.5c worktree. Rows distinguish merged code, current unmerged moves, remaining mixed persistence, and target proposals. Target tree nodes still labeled proposed are not evidence that those packages exist. This does not infer source behavior from diagrams or package names. Direct source, imports, migrations, and transaction tests control this disposition.

## Structure principles

- Organize by business context first, cohesive capability second, and technical layer only where the code needs it.
- Keep a small context flat. Introduce `domain`, `application`, `sqlite`, or `platform` subpackages only to make a real dependency boundary or capability easier to test and own.
- Keep a capability's cohesive types, use case, and tests discoverable; do not move a file because its name resembles a target folder.
- The owning context defines invariants and transaction semantics. A shared adapter may provide SQLite handles and migration ordering without owning every table.
- Keep public contracts explicit and small. Avoid generic `common`, `utils`, `services`, or `models` buckets, generated-empty layers, service locators, reflection DI, and a custom plugin framework.
- Prefer characterization tests and short-lived compatibility facades over a big-bang move. Every facade must name its active consumers, reason to retain it, and measurable removal criteria; a facade without those is not part of the target.

## Concrete target tree

This is a proposed physical navigation after migration, grounded in the source snapshot above. Read each level as **Context → Capability → technical layer only when justified**. It is not a mandatory package scaffold: keep a capability flat where its types and use cases remain cohesive, and introduce a technical layer only for an actual adapter, transaction, or platform boundary. The matrix maps current source candidates, consumer dependencies, and invariants. Tree nodes are candidate destinations, not a checklist for creating empty folders. Tests move with the production capability unless they are shared infrastructure. In the matrix, source paths are relative to `internal/` unless they begin with another repository-root path; target package names are also relative to `internal/`. A brace or wildcard expression groups files listed by capability and is not a literal package name.

The collaboration/data-flow view and Go import view are separate: business collaboration does not imply an import. [02-context-ownership](02-context-ownership.md) describes ownership and collaboration; [03-target-architecture](03-target-architecture.md) separately defines allowed import direction. The dependency column below names contracts/adapters a capability consumes; it does not redraw business-flow arrows.

```text
cmd/agentctl/                                  CLI and service delivery edge
internal/
├── app/                                       composition, process host, worker lifecycle
├── identity/
│   ├── host/                                  machine ID, OS and platform facts
│   ├── installation/                          installation ID and durable Core binding
│   ├── sqlite/                                installation identity repository and shared transactional reader
│   ├── enrollment/                            attempt lifecycle and retry/resume rules
│   │   └── sqlite/                             attempt rows; same-context identity validation
│   └── credentials/                           protected credential-store interface/adapters
├── inventory/ (flat observation, normalization and reconciliation files)
│   ├── docker/                                Docker API provider and Docker-specific evidence
│   └── sqlite/                                inventory-owned rows and snapshot transactions
├── monitoring/
│   ├── profiles/                              profile definitions, registry and resolution
│   ├── matching/                              selectors, recommendation and required capabilities
│   ├── endpoints/                             pluggable endpoint strategy and diagnostics
│   └── templates/                             match evaluation only; definitions remain in Configuration
├── configuration/
│   ├── desiredstate/                          accepted instances and desired revisions
│   ├── remoteconfig/                          Core snapshot, local/Core authority and revisions
│   ├── proposals/                             proposal lifecycle and approval decisions
│   ├── templates/                             template definitions, ownership and revisions
│   ├── materialization/                       CAS plan and desired-state projection
│   ├── reconciliation/                        desired-to-effective decisions and apply requests
│   ├── rendering/                             conditional: deterministic merge if imports justify ownership
│   └── sqlite/                                context-owned repositories and atomic cross-capability writes
├── runtime/
│   ├── bootstrap/                             first-run setup and recovery entrypoints
│   ├── artifacts/                             conditional; default to capability-local artifact behavior
│   ├── dependencies/
│   │   ├── catalog/                            managed dependency definitions
│   │   ├── node_exporter/                       Node Exporter capability and platform behavior
│   │   ├── windows_exporter/                    Windows Exporter capability and MSI/SCM behavior
│   │   └── libre_hardware_monitor/              LHM artifact, config, task/process and health behavior
│   ├── collector/
│   │   ├── capabilities/                       pinned-binary capability catalog/query
│   │   ├── configuration/                      compile and validate candidate configuration
│   │   └── activation/                          single-owner activation, readiness, LKG/recovery
│   ├── opamp/                                 managed Supervisor provisioning/config/status adapter
│   ├── service/                                Agent install, Windows Service/systemd lifecycle
│   └── process/                                child-process identity, stop and cleanup
├── health/
│   ├── snapshot/                               compatible status snapshot composition
│   ├── evidence/                               owner-sourced assessment and freshness
│   └── reporting/                              local diagnostics and supported reporting ports
├── operations/
│   ├── command/                                versioned request, registry and executor
│   ├── handlers/                               diagnostics, container, dependency, restart
│   ├── dockeradapter/                          exact-target inspect/restart adapter
│   ├── nats/                                   authenticated JetStream session and result publish
│   └── sqlite/                                 execution journal, audit and result outbox
├── platform/
│   ├── sqlite/                                 connection policy
│   │   └── migrate/                            generic ordered migration executor
│   └── events/                                 bounded post-commit advisory notifications
├── coreapi/                                     authenticated Core HTTP protocol adapter
├── opampserver/                                separate Go OpAMP protocol server/service
└── mockcore/                                   replaceable contract test service, test-only authority

configs/                                        Collector/profile inputs, not generated state
contracts/                                     versioned Agent wire schemas; keep shared contracts in docs/shared
infra/                                          isolated Docker/OTLP testbeds
docs/                                           architecture, contracts, plans, runbooks and evidence
```

At merged PR-09.5b integration `0f1addd`, `internal/identity/{host,installation,enrollment,credentials}` and `internal/inventory/{docker,sqlite}` contain moved production code and tests. Inventory observation/reconciliation remains flat; no extra observation package was needed. In PR-09.5c executable-source commit `d04c69d4bb73c000a24ac797f4846a7a3e22b5fe`, Configuration owns proposal lifecycle, reconciliation workflows and attempt/evidence persistence in `internal/configuration/sqlite`; schema callbacks 6, 9, 12 and 13 remain registered in order by the existing `agentstate.ensureSQLiteSchema` composition. Runtime owns immutable Collector candidates and candidate-application contracts under `internal/collectorruntime/contracts`. `internal/agentstate/sqlite_candidate_outcome.go` is the narrow Runtime journal reader; `CandidateOutcomeTx` reads through the Configuration transaction snapshot. `internal/agentstate/sqlite_reconciliation_adapter.go` is now only a compatibility adapter that maps pending Runtime recovery state to Configuration recovery evidence and delegates repository operations. It no longer owns reconciliation SQL. Monitoring remains in the existing `internal/monitoringprofile` and `internal/integration` packages, and the pinned Runtime catalog remains in `internal/collectorruntime`; these are current source locations, not evidence that proposed `internal/monitoring` or `internal/runtime` trees exist. `runtime/artifacts` and shared dependency `adapters` are conditional seams: keep artifact handling inside each capability unless at least two current callers share the same behavior and can use one small interface. Do not create those directories during a mechanical move.

Protocol and test ownership stay outside the seven business contexts:

```text
internal/opampserver/                       Go OpAMP server protocol/service adapter
internal/mockcore/                          replaceable Core contract test service
internal/operations/testsupport/mockcore/   focused NATS binding/authorization fixture
contracts/operations/                       Agent-local versioned wire contracts
infra/                                      Docker, Collector and OTLP testbed orchestration
```

`internal/opampserver` translates between Core contracts, Agent identity/binding and the OpAMP protocol. It is neither Runtime's managed Supervisor client nor an Agent business context. The full Mock Core is test infrastructure and never owns production business policy. Keep the Operations Mock Core fixture separate while it serves narrower broker scenarios; consolidate only if doing so removes duplicate behavior without coupling production packages. `docs/shared/` remains the authority for cross-repository contracts and standards; local Agent schemas do not silently become shared contracts.

There is no target `internal/state` or generic state hub. `internal/platform/sqlite/migrate` is the context-independent ordered migration executor. `internal/agentstate` remains a transitional multi-context persistence adapter and currently registers migrations. PR-09.5b moved callbacks 2, 5, 10 and 11 to their owners. In the PR-09.5c worktree, Configuration defines callbacks 6, 9, 12 and 13; `agentstate.ensureSQLiteSchema` still composes versions 1–14 in their existing order. Each context owns its schema meaning, repository and transaction; final composition-root registration remains a later slice, and the platform coordinator imports no business context. Preserve every existing atomic transaction. Migration 1's historical non-transactional behavior is also preserved; do not describe every migration as atomic. Retire `agentstate` only when all active consumers use owner repositories, migration-upgrade/reopen fixtures pass, and state-path, lifecycle-lock and activation-journal behavior have explicit owners.

#### Existing SQLite migration disposition

The version numbers and callbacks below are historical identities, not a proposed new migration sequence. Keep each callback and its current transaction boundary intact while transferring table semantics to the owning context. The composition boundary owns only registration order; `platform/sqlite/migrate` validates and executes registrations and must not import these owners.

| Version | Existing name and source | Target semantic owner / disposition | Invariants and boundary notes |
|---|---|---|---|
| 1 | `Agent state and activation journal`; `agentstate/sqlite_schema.go` | `schema_migrations` metadata → technical migration coordinator; `activation_journal` → Runtime Collector activation; `agent_state` JSON payload → legacy persistence compatibility during extraction, with no new target `state` package | Keep the historical callback and non-transactional behavior. Retain the legacy payload only while import/recovery consumers exist; define its removal against those consumers and recovery fixtures. |
| 2 | `Installation identity`; owner callback now `identity/sqlite/migrations.go` (ordered registration still in `agentstate/sqlite_schema.go`) | Identity installation repository (`identity/sqlite`) | Preserve singleton initialization, stable installation ID, and migration's current atomic transaction. |
| 3 | `Core Gateway snapshot`; `agentstate/sqlite_core_gateway_schema.go` | Configuration remote configuration (`configuration/sqlite`) | The accepted Core snapshot and revision/authority semantics remain unchanged; keep table creation and migration record atomic. |
| 4 | `Core Gateway runtime`; `agentstate/sqlite_core_gateway_schema.go` | Configuration remote-config state and Runtime activation attribution share fields in the current record; assign field-level read/write ownership from existing call sites before extraction | Preserve the whole migration transaction and seeding from existing state. Do not change authority, last-good endpoint, activated/applied revision, or generation semantics while clarifying the ports. |
| 5 | `Enrollment attempts`; owner callback now `identity/enrollment/sqlite/migration.go` (ordered registration still in `agentstate/sqlite_schema.go`) | Identity enrollment (`enrollment/sqlite`) | Preserve binding validation and attempt-state atomicity, retry/resume, and uncertain-outcome semantics. |
| 6 | `Integration desired state`; PR-09.5c worktree: `configuration/sqlite/desired_state_schema.go` (moved from `agentstate/sqlite_integration_schema.go`) | Configuration desired state (`configuration/sqlite`) | Preserve instance IDs, expected-revision checks, stored representation, and no-op revision behavior; DDL and transaction body are unchanged. |
| 7–8 | `Candidate activation mapping` and `Candidate attempt history`; `agentstate/sqlite_candidate_schema.go` | Runtime Collector activation journal/history | Preserve generation attribution, candidate digest/fingerprint, outcomes and recovery evidence; keep the existing migration ordering and transaction scope. |
| 9 | `Reconciliation diagnostics`; PR-09.5c worktree: `configuration/sqlite/reconciliation_schema.go` (moved from `agentstate/sqlite_reconciliation_schema.go`) | Configuration reconciliation diagnostics, with Runtime apply outcomes represented through its existing narrow result contract | Preserve target/attempt history and status/revision attribution; DDL and transaction body are unchanged; the app host remains a worker coordinator, not the decision owner. |
| 10–11 | `Discovery inventory` and `Inventory write revision`; owner callbacks now `inventory/sqlite/migration.go` (ordered registration still in `agentstate/sqlite_schema.go`) | Inventory SQLite repository | Preserve completeness, provider status, inventory revision/write revision and atomic snapshot updates. |
| 12 | `Monitoring proposals`; PR-09.5c worktree: `configuration/sqlite/proposal_schema.go` (moved from `agentstate/sqlite_proposal_schema.go`) | Configuration proposals | Preserve proposal history and the single transaction that approves the proposal and transitions desired state. |
| 13 | `Integration templates`; PR-09.5c worktree: `configuration/sqlite/template_schema.go` (moved from `agentstate/sqlite_template_schema.go`) | Configuration templates and materialization | Preserve owner/tombstone/ID-reuse rules and the cross-revision compare-and-swap batch transaction. |
| 14 | `Operations execution and outbox`; `agentstate/sqlite_operations_schema.go` | Operations SQLite journal, audit and result outbox | Preserve durable idempotency, execution/audit/outbox atomicity, replay, and unknown-outcome semantics. |

Version 4 is a concrete extraction seam to resolve from source call sites, not authority to invent a new policy or move a write across transactions. If the current record must be split into Configuration-owned and Runtime-owned repositories, preserve any cross-owner atomic write with one explicitly owned transaction adapter and narrow ports; do not let either context import the other's private SQLite package.

### Package disposition matrix by context and capability

Each row names a responsibility, source candidates, intended package boundary, dependencies and invariants. The source audit establishes what exists; the target column records a proposal, not a claim that the package already exists. Movement is incremental: split mixed files/models by semantic owner only after callers and translations are characterized. A package may remain at its current path if moving it does not improve ownership.

| Context navigation | Capability groups in this proposal |
|---|---|
| [Identity](#identity) | Host and installation identity, enrollment/binding, protected credentials |
| [Inventory](#inventory) | Provider observations, Docker adapter, inventory snapshots and persistence |
| [Monitoring](#monitoring) | Profiles, matching/recommendations, endpoint diagnostics, template matching |
| [Configuration](#configuration) | Desired and remote state, proposals, templates, materialization, reconciliation, rendering |
| [Runtime](#runtime) | Bootstrap/artifacts, managed dependencies, Collector, OpAMP Supervisor, Agent service and processes |
| [Health](#health) | Health evidence snapshots and reporting |
| [Operations](#operations) | Typed commands, adapters, audit and result delivery |
| [Supporting boundaries](#supporting-technical-and-protocol-boundaries) | Composition/CLI, SQLite migrations, Core/OpAMP protocol services and test infrastructure |

#### Identity

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Host identity: machine IDs, OS/platform facts, release metadata | moved from `identity/machine*.go`, `identity/platform*.go`, `identity/os_release.go` to `identity/host` | OS adapters only; stable identity inputs, Windows/Linux behavior, and platform-specific file isolation. |
| Installation identity: parse/create ID and bind the durable installation record | moved ID/model code to `identity/installation`; SQLite repository remains `identity/sqlite` and consumes the installation-owned model | Host identity and SQLite repository port; stable installation ID, Core-bound Node identity and atomic initialize/read semantics. `identity/sqlite.ReadInstallationID` is also used by enrollment persistence to validate binding within the enrollment transaction. |
| Enrollment and binding: attempt lifecycle, uncertain outcomes, retry/resume | moved from `enrollment/{model,service}.go` and `enrollment/sqlite/*` to `identity/enrollment` and `identity/enrollment/sqlite`; `agentstate` remains the shared transaction adapter | Installation identity, Core enrollment port and protected credential port; preserve persisted uncertainty, retry classification, binding/attempt atomicity and Core response semantics. Enrollment owns attempt decisions; Identity owns the stable binding value. Keep installation validation inside the same enrollment SQL transaction through a narrow same-context identity query; do not introduce a cross-context persistence dependency or generic state package. |
| Credential protection: store, replace and revoke secret material | moved from `credentialstore/store*.go` to `identity/credentials` | OS ACL/DPAPI or Linux file permissions; the store receives secret references/material only at its boundary, and secrets never enter logs, SQLite rows, CLI output or diagnostics. |

`coreapi` is currently a narrow shared HTTP adapter, not all Identity business logic. Keep its transport boundary explicit; split enrollment requests from control/configuration endpoints only after the consumed contracts and DTO translation are mapped. `coreconfig.GatewaySnapshot` authority/revision semantics belong to Configuration's `remoteconfig`, not host Identity.

#### Cross-context model ownership and translation

These source types are currently shared or located outside their target owner. Treat the destination owner as the canonical business model; other contexts consume a narrow read model or translate at their adapter/contract boundary. Do not preserve one ambiguous shared `integration` or `discovery` model merely to avoid a small explicit mapping.

| Current model/source | Canonical target owner | Other consumers and translation boundary | Invariants |
|---|---|---|---|
| `inventory.ResourceObservation`, `ProviderSnapshot`, and `Resource` (moved from `discovery`) | Inventory observation/inventory model | Monitoring reads inventory evidence through a query contract; Configuration materialization consumes snapshot identity/revision, not Inventory persistence types. | Provider-scoped `resource_id`, optional stable `workload_id`, completeness/provenance and historical identity remain distinct. |
| `integration.IntegrationDefinition`, registry and profile-resolution types | Monitoring profiles | Configuration receives a resolved monitoring intent through a contract; no profile matching or Collector activation side effect crosses with it. | Profile schema/version and extensibility remain stable; pinned capability support comes from Runtime's narrow query. |
| `integration.IntegrationInstance` and `ResourceBinding` | Configuration desired state | Monitoring supplies resolved intent; Inventory supplies observed identity. Translate at the Configuration input boundary and preserve persisted representation/revision semantics. | Stable desired IDs, explicit binding identity and no-op revisions remain unchanged. |
| `integration.IntegrationTemplate` and template ownership/revision fields | Configuration templates | Monitoring's `TemplateMatchResult`/`TemplateMatchReport` is evaluation evidence consumed through a narrow contract. | Owner, status/tombstone, ID reuse, deterministic setting resolution, and template/Inventory/desired revision CAS remain intact. |
| `configuration/proposals.Proposal`, `ProposalEvidence`, and proposal status | Configuration proposals | Inventory supplies observed evidence; Monitoring supplies a recommendation; Configuration owns lifecycle and persistence. | Migration 12 history and approval plus desired-state transition remain one Configuration-owned transaction. Proposal creation/dismissal keep their existing separate revision-checked writes. |
| `collectorruntime.Capability` and manifest-derived capability set | Runtime Collector capability catalog | Monitoring asks a narrow supports/query contract; it does not import Runtime's mutable catalog implementation or keep a duplicate catalog. | The pinned Collector binary/manifest remains the single capability source. |
| `coreconfig.GatewaySnapshot` and accepted remote revision | Configuration remoteconfig | `coreapi` owns wire DTO/authentication translation; Identity owns enrollment/binding. | Preserve approved Core/local authority and revision behavior; unresolved policy remains separate. |

#### Inventory

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Observation coordination and normalization | moved from `discovery/{coordinator,types,normalize,reconcile}.go` into flat `inventory` package; `inventory/observation` is not introduced without a boundary need | Provider contract, Identity association and Monitoring read port; complete snapshots only, provider-scoped IDs, provenance, missing/history semantics. `reconcile.go` resource absence logic stays here; proposal projection does not. |
| Docker discovery | moved from `discovery/docker/provider.go` to `inventory/docker` | Docker API plus observation contract; labels/endpoints/network metadata remain evidence, incomplete or failed scans never delete inventory. |
| Inventory persistence | moved `discovery/sqlite/{store,inventory}.go` and version 10–11 callbacks into `inventory/sqlite` at PR-09.5b | `agentstate` still provides shared database handles and ordered registration; preserve atomic snapshot, completeness, write revision and change evidence. |

#### Monitoring

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Profile definitions and resolution | Current: `integration/{integration,registry,resolver}.go`, `integration/redis/definition.go`, `monitoringprofile/{profile,loader,docker_config}.go`; target: `monitoring/profiles` when the capability move is independently justified | Inventory observation/query port and Runtime capability-query contract; exact definition version, profile extensibility, no generic-discovery edits for new profiles. Redis is the first concrete profile. Docker label key values now originate in Inventory and are re-exported as Monitoring aliases for compatibility. |
| Resource matching and recommendation | Current: `integration/{matching,matching_engine}.go`, `monitoringprofile/diagnostics.go`; target: `monitoring/matching` | Inventory projection, profile registry and Runtime capability query; explicit match and required-capability outcomes; returns recommendation/monitoring intent without creating or approving Configuration state. `integration.CapabilityAssessor` remains the consumer port; `cmd/agentctl` adapts the Runtime catalog and is covered by a pinned-catalog characterization test. |
| Endpoint strategy and diagnostics | `monitoringprofile/{diagnostics,docker_config}.go`, endpoint strategy portions of `integration/redis/definition.go` → `monitoring/endpoints` only if provider/platform variation remains after characterization | Provider evidence and profile contract; report missing, ambiguous, resolved and unreachable separately. Never equate discovered endpoint with reachability; selection stays deterministic and receiver/lifecycle work stays in Runtime/OTel. |
| Template match evaluation | `integration/template_matching.go` → `monitoring/templates` (evaluation only) | Reads Configuration-owned template definitions through a narrow query contract; emits match evidence, not template state or desired configuration. |
| Receiver capability query | Current source `collectorruntime/{capability,manifest}.go` → Runtime-owned `runtime/collector/capabilities`; current adapter is `cmd/agentctl/catalogCapabilityAssessor` | Pinned Collector binary/manifest is the sole catalog source. `integration.CapabilityAssessor` is the narrow Monitoring query contract. The PR-09.5c worktree test verifies Redis support, unsupported IDs and exact platform/artifact/manifest evidence; CI artifact probing remains the binary-level source of truth. |

Template matching is a Monitoring computation over Configuration-owned template definitions. `integration/template.go` currently mixes template policy/binding models with integration types; split owner-specific models and translate through the reviewed contract, rather than moving the whole file or sharing an unowned `integration` model.

#### Configuration

| Capability and responsibility | Current source → intended package | Dependencies / transaction invariants to preserve |
|---|---|---|
| Desired state: accepted Integration instances and revisioned reads/writes | Current: `configuration/{integration.go,sqlite/integration.go}`, `integration.IntegrationInstance`, `agentstate/sqlite_configuration.go`; migration 6 now `configuration/sqlite/desired_state_schema.go` in PR-09.5c → `configuration/desiredstate` plus the Configuration SQLite adapter | Monitoring's resolved intent, SQLite repository; expected revision checked in the same write, stable IDs, no-op revisions remain no-op. Split shared `integration` structs by owner with explicit translation. |
| Remote configuration: authenticated Core Gateway snapshot, accepted revision and effective authority | `coreconfig/gateway.go`, `agentstate/sqlite_core_gateway*.go`, Core control DTO use in `coreapi/control.go` → `configuration/remoteconfig` plus Core API adapter | Core API authentication and local/bootstrap input; preserve approved Core/local authority and revision rules. Do not decide still-open policy such as namespace relinquishment here. |
| Proposals: candidate creation, validation, dismissal and approval decision | Current PR-09.5c worktree: `configuration/proposals/{proposal,proposal_validation,approval}.go`, `configuration/sqlite/{proposal,approval,proposal_schema}.go`; proposal CLI remains under its existing `cmd/agentctl` command path | Inventory observation plus Monitoring recommendation; proposal lifecycle is Configuration-owned. Preserve migration 12, proposal identity/revisions/history, and one transaction for approval plus desired-state transition. Proposal creation and dismissal retain their existing separate revision-checked writes. `discovery/reconcile.go` was moved to Inventory in PR-09.5b as `inventory/reconcile.go`. Keep approval atomic; do not replace it with event-driven writes or independently committed repositories. |
| Templates: reusable configuration definitions, authority, revisions and persistence | `integration/template.go` models, `configuration/template.go`, `configuration/sqlite/template.go`, `agentstate/sqlite_template_schema.go` → `configuration/templates` | Monitoring match results, desiredstate and Inventory revision port; preserve owner, tombstone/ID reuse rules and deterministic setting resolution. Matching itself stays in Monitoring. |
| Materialization: plan template bindings into desired instances | `configuration/materialization.go`, `configuration/sqlite/materialization.go` → `configuration/materialization` | Templates, Inventory snapshot/revision and desired state; preserve one CAS over Inventory, template and desired revisions, stable generated IDs, and atomic batch writes. |
| Reconciliation: decide desired-to-effective convergence and request apply | Current PR-09.5c source: controller in `configuration/reconciliation`; workflow DTOs/errors in `configuration/reconciliation/contracts`; Runtime immutable candidate/application contract in `collectorruntime/contracts`; schema callback in `configuration/sqlite/reconciliation_schema.go`; attempt/evidence adapter in `agentstate/sqlite_reconciliation_adapter.go`, delegating to the shared `agentstate.SQLiteStore` | Configuration desired state, Monitoring Integration resolution, Runtime compiler/validator and Runtime candidate/apply contracts; preserve periodic/startup recovery, bounded retry, status/revision attribution, and the single transaction that reads Runtime activation-journal evidence while updating Configuration attempt state. The OD-05 decision-workflow move is complete; underlying table methods remain in `SQLiteStore` until an explicit transaction seam preserves cross-table atomicity. `internal/app` owns only worker lifecycle.
| Rendering: merge deterministic base/profile/OS/vendor/local layers and write candidate input | `config/{types,merge,renderer,writer,validator,readiness,metrics_pipeline}.go` → `configuration/rendering` only if ownership/import tests confirm; otherwise retain focused `internal/config` | Configuration layers, Identity values, Monitoring intent; deterministic merge, validation before activation, explicit provenance and no arbitrary remote YAML. Runtime's Collector compiler/capability validation remains separate. |

Configuration is intentionally split by capability, not one directory of generic services. The current `integration` and `discovery` packages mix policy with profile/evidence models; do not move a file wholesale when it contains multiple owners. The proposed `configuration/sqlite` is the technical adapter boundary for Configuration-owned repositories and cross-capability atomic writes; keep business decisions in their capability packages. The proposal/desired-state transaction is the strongest cross-capability atomic boundary and must remain one SQLite transaction after extraction. Split the SQLite adapter further only if transaction ownership stays singular and repository imports become clearer.

#### Runtime

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Bootstrap: first install, configuration preparation and recovery entrypoints | `bootstrap/{bootstrap,install,command,config_activation,config_recovery}.go` → `runtime/bootstrap` | CLI/Application Host and artifact/service ports; idempotent install, no change to command behavior or operator configuration ownership. |
| Artifact management: pinned download, checksum/verification, staging and publication | `bootstrap/{artifact,download,publish*}.go`, `agentinstallation/binary_copy.go`, and capability-local dependency artifact/download files → `runtime/artifacts` only if a shared artifact interface is proven | Versioned manifests and OS filesystem/service adapters; checksum and atomic replacement remain mandatory. Keep per-dependency acquisition local unless two real callers share policy and failure semantics. Split `supervisor_artifact.go` only if artifact and OpAMP configuration responsibilities need separate owners. |
| Managed dependency catalog | `dependency/{catalog,types}.go` → `runtime/dependencies/catalog` | Static supported dependency definitions; one canonical catalog, stable CLI names and no speculative plugin loading. |
| Dependency provisioning/lifecycle: Node Exporter | `dependency/node_exporter/*.go` → `runtime/dependencies/node_exporter` with platform files retained inside the capability unless a shared adapter seam is proven | Catalog, artifact ports, systemd/process adapters; preserve managed ownership, idempotent install/enable/reconcile/teardown, host privilege and no removal of unowned installs. |
| Dependency provisioning/lifecycle: Windows Exporter | `dependency/windows_exporter_*.go` (currently root-level files) → `runtime/dependencies/windows_exporter` | Catalog, MSI/artifact and Windows SCM adapters; preserve preflight/admin checks, ownership markers, service recovery and safe uninstall. |
| Dependency provisioning/lifecycle: LibreHardwareMonitor | `dependency/libre_hardware_monitor/*.go` → `runtime/dependencies/libre_hardware_monitor` | Catalog, artifact, Windows task/firewall/process adapters; preserve secret-free config, firewall/task ownership, health checks and cleanup. |
| Shared dependency platform primitives | `dependency/{service,process}.go` and any truly shared helpers → remain within their consuming capability initially; `runtime/dependencies/adapters` only after importer audit proves a shared boundary | Capability packages; platform-neutral contracts and Windows/Linux build constraints. Do not force exporter-specific lifecycles into a generic lifecycle framework. |
| Collector catalog, compile and validate | `collectorruntime/{capability,manifest,compiler,validation}.go` → `runtime/collector/{capabilities,configuration}` | Monitoring query, Configuration rendered input, Identity platform; pinned manifest is canonical and candidate validation occurs before activation. |
| Collector activation, readiness, rollback and recovery | `agentlifecycle/{candidate*,configuration,rollback,activation_recovery,runtime*,teardown*,baseline}.go`, `supervisor/{supervisor,lifecycle,readiness,child*}.go` → `runtime/collector/activation` and `runtime/process` only where process ownership is distinct | Configuration apply request, Collector adapter and durable activation journal; one lifecycle owner, generation evidence, readiness, LKG and terminal-state guard. Preserve controlled legacy fallback during OpAMP Supervisor migration and never run two lifecycle owners concurrently. |
| Managed OpAMP Supervisor provisioning and status | `opampsupervisorconfig/config.go`, `agentlifecycle/supervisor_runtime.go`, `bootstrap/supervisor_artifact.go` → `runtime/opamp` | Runtime artifact, Collector lifecycle and Core control adapter; Supervisor is a managed dependency/protocol participant, not the OpAMP Server and never a second simultaneous Collector owner. |
| Agent service installation and native lifecycle | `agentinstallation/{installer,layout,service_default*,windows_service*,systemd*,remover*,uninstall_guard}.go` → `runtime/service` | Bootstrap/artifact and platform service managers; preserve installation layout, state, Windows Service/systemd behavior, explicit migration and uninstall ownership. |
| Process identity and cleanup | `processowner/*`, `supervisor/{child,child_unix,child_windows}.go`, process portions of `agentlifecycle` → `runtime/process` | Native process APIs and lifecycle coordinator; bounded shutdown, process-tree cleanup and no duplicate process owner. |

The existing `internal/dependency` has 72 production Go files: 22 at the package root (including catalog/types and Windows Exporter code), 23 in `node_exporter`, and 27 in `libre_hardware_monitor`. The source identifies those three dependency capabilities, but not a separate Windows Exporter directory yet. Move the root-level Windows Exporter files into its capability only after characterization confirms which catalog/shared types they use. Introduce shared `catalog`, `provisioning`, `artifacts`, `lifecycle`, or `adapters` subpackages only where imports and duplicated behavior prove cohesion. The proposed catalog is justified by the existing catalog files; shared provisioning/artifact/lifecycle packages are not yet justified merely by parallel filenames. Do not create empty package layers to match the illustrative tree.

#### Health

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Health snapshot and evidence model | `agenthealth/{snapshot,assessment,owner_evidence,sanitize}.go` → `health/{snapshot,evidence}` | Narrow owner evidence ports from Identity, Inventory, Runtime and Collector diagnostics; timestamps/freshness/redaction; missing evidence is UNKNOWN, readiness is not delivery. |
| Health transports/reporting | `agenthealth/{client,http,server,reporter}.go` → `health/reporting` | Existing local status and approved reporting contracts only; no custom Gateway probe or invented Core endpoint/schema. |

#### Operations

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Operations command/registry/executor | `operations/{operation,executor}.go` → `operations/command`; wire schemas stay in `contracts/operations/*.json` | Core-authorized request contract and handler ports; exact-target validation, idempotency, duplicate prevention and durable UNKNOWN outcomes. |
| Operations handlers and Docker adapter | `operations/handlers/*.go`, `operations/dockeradapter/runtime.go` → `operations/handlers` and `operations/dockeradapter` | Health, Inventory and Runtime narrow ports; diagnostics/inspect/restart only; local opt-in, exact immutable container ID and post-restart verification remain. |
| Operations transport and persistence | `operations/nats/transport.go`, `operations/sqlite/repository.go`, `agentstate/sqlite_operations_schema.go` → existing `operations/nats` and `operations/sqlite` adapters | Core-provisioned binding; preserve credential invalidation, monitor independence, one transaction for execution/audit/outbox, replay and unknown-outcome rules. |

#### Supporting technical and protocol boundaries

| Boundary | Current source → intended boundary | Dependencies / invariants to preserve |
|---|---|---|
| Composition and worker host | `app/runtime.go`, `cmd/agentctl/composition.go` and command wiring → `internal/app` / `cmd/agentctl` | App starts/stops workers and composes context ports; contexts decide business workflows; CLI flags/output/exit codes remain compatible. |
| SQLite connection and migration coordination | Current: `platform/sqlite/sqlite.go`, new `platform/sqlite/migrate/{doc.go,migrate.go}`, `agentstate/sqlite_schema.go`, and context `*/sqlite` repositories. Target: keep connection policy and ordered execution in `platform/sqlite`; move table semantics and migration callbacks alongside context-owned repositories, with ordered registration in App/composition. | One database; one coordinator; schema versions 1–14 and order unchanged; no context imports by platform; per-operation atomic transactions unchanged. No replacement `state` package. |
| Advisory events | `platform/events/dispatcher.go` → `platform/events` | Post-commit wakeups only; SQLite and startup/periodic recovery remain authoritative. |
| Core HTTP protocol adapter | `coreapi/{enrollment_client,control}.go` → retain `internal/coreapi` as a shared protocol adapter; split client files only if consumer contracts require distinct seams | Identity enrollment, Configuration remoteconfig and Runtime status call sites; keep transport DTOs at edge and do not merge client protocol with business policy. Avoid moving it into a generic adapters bucket without a demonstrated dependency improvement. |
| OpAMP Server | `opampserver/{server,config}.go` → retain `internal/opampserver` as separate service adapter | Core API, Identity binding and OpAMP protocol; not Runtime's managed OpAMP Supervisor and not an Agent business context. |
| Full Mock Core | `mockcore/*` → retain `internal/mockcore` test infrastructure | Versioned Core/OpAMP contract fixtures; replaceable by endpoint/config, never production policy authority. |
| Operations Mock Core | `operations/testsupport/mockcore/*` → retain as narrow Operations test fixture | NATS binding/authorization scenarios; consolidate with full Mock Core only if it reduces duplicated test behavior without coupling production packages. |

### Migration dependency order

Use this as the capability order for the canonical [PR-09.5 migration plan](../../../plans/v0.1.4a-next/pr-09.5-architecture-consolidation.md): characterize migrations, transactions and host loops first; extract Identity and Inventory repositories while preserving existing schema order; establish Runtime's capability query before moving Monitoring consumers; then move Configuration desired state/proposals/templates/materialization/reconciliation; split remaining Runtime dependency, Collector, OpAMP Supervisor, service and process capabilities; move Health/Operations evidence ports; finally thin CLI/App wiring and remove facades whose exit criteria have passed. Keep each step independently testable and rollbackable.

## Documentation navigation and preservation

`docs/README.md` is the actual repository documentation entrypoint; there is
no `docs/local/README.md` in the audited tree. `docs/shared/` is a separate
pinned submodule for cross-repository contracts and standards. Preserve both
roots and historical documents. The directory sketch below reflects existing
paths and does not ask for a move.

```text
docs/
├── README.md                            repository navigation and release entrypoint
├── ARCHITECTURE.md                       implemented PoC overview; target link at top
├── local/
│   ├── architecture/v0.1.4a-next/        release behavior, contracts and index
│   │   └── consolidation/                PR-09.5 source audit, ownership, target and tree
│   ├── adr/                              cross-release decisions
│   ├── archive/                          retained historical docs
│   ├── development/                      local coding-policy supplement
│   ├── plans/v0.1.4a-next/               checkpoint plans and release progress
│   ├── requirements/v0.1.4a-next/        scope, acceptance and unresolved contracts
│   ├── runbooks/v0.1.4a-next/            operator procedures
│   ├── testing/ and verification/         testbed guidance and evidence
│   └── guides/, changelog/, examples/, handoffs/, roadmap/
└── shared/                               pinned cross-repository docs submodule
```

### Canonical sources, duplication and archival guidance

| Document | Canonical responsibility | Duplication / disposition |
|---|---|---|
| `docs/README.md` and `docs/ARCHITECTURE.md` | Repository entrypoint and implemented PoC baseline, respectively; both link to the v0.1.4a-next target. | Keep short and navigational. Do not copy the release capability matrix or target package tree into them. |
| `architecture-overview.md` | Product behavior, scope, release constraints and acceptance scenarios. | Section 12 is now explicitly marked as a superseded package map and points here. Sections 16 and 20 retain control-plane and feature-scope questions; OD-06 authority/revision preservation is clarified without resolving those separate policies. Preserve behavior scenarios and decision history. |
| `consolidation/01-current-source-audit.md` | Source facts at immutable PR-09 merge `b2599b8`, import observations, migrations and atomicity. | Canonical pinned-baseline audit, not a continuously current inventory. Do not duplicate package inventories in other architecture docs; refresh at each implementation baseline. |
| `consolidation/02-context-ownership.md` | Approved OD-01–OD-06 logical ownership and collaboration flow. | Canonical context owner map. Keep package destinations in this document only as short links, not a second tree. |
| `consolidation/03-target-architecture.md` | Target context contracts, collaboration flow and separate Go-import direction. | Canonical approved interaction/dependency diagrams. The context-boundaries document remains a compact logical summary. |
| `consolidation/04-source-structure.md` | Approved capability tree, source-to-target disposition, boundaries, retention/removal criteria and documentation audit. | Canonical physical target guidance; derive the migration plan from this file rather than maintaining duplicate move maps. |
| `plans/v0.1.4a-next/pr-09.5-architecture-consolidation.md` | Ordered implementation slices, gates, characterization, verification and rollback. | Canonical migration execution plan; avoid repeating full ownership tables here. |
| `verification/v0.1.4a-next/pr-verification/` | Evidence and checkpoint outcomes. | Canonical claims about what is implemented/tested; plans and architecture diagrams are not verification evidence. |
| `context-boundaries.md`, `source-structure.md`, `state-and-event-model.md`, `opamp-integration.md` | Logical context summary, historical package proposal, persistence invariants and OpAMP runtime/contracts. | Keep targeted, link to the consolidation set for current placement. `source-structure.md` is already marked historical and superseded; retain it for design history, not as migration input. |
| `consolidation/README.md` and `architecture/v0.1.4a-next/README.md` | Review-set and release navigation. | The former indexes the five planning artifacts; the latter indexes all release docs. Avoid copying their explanations into each other. |

**Capability-based navigation:** the architecture index now links each context to the canonical ownership map, its capability section here, and the release behavior/acceptance overview. Keep the package tree only here; do not duplicate it in the README.

**Status consistency:** OD-01 through OD-06, the physical map, the complete target architecture and the ordered migration plan are approved. Keep this document the sole canonical package tree; other indexes link to it without copying its contents.

**Archival opportunity:** do not move records as part of source migration. After architecture approval, a separate documentation-only cleanup may move the superseded unversioned package tree in `source-structure.md` and stale implementation checklists to `docs/local/archive/`, or leave them in place with their supersession banners. Before moving, verify inbound links, dated decision provenance and policy-history needs; preserve links or add a short archival index. Do not archive release contracts, verification evidence, runbooks, or historical decisions that explain a still-supported migration path. Keep the long `architecture-overview.md` behavior scenarios, acceptance matrix and decision history; archive only its clearly superseded package-map subsection in a later, separate docs-only change if the audit confirms no unique policy text is lost.

### Compatibility and package-retention decisions

| Existing package or facade | Retention reason during migration | Removal / final-location criteria |
|---|---|---|
| `internal/agentstate` | Current SQLite initialization, legacy state import, activation journal, cross-process locks and migration registration are coupled here. The ordered technical executor now exists at `internal/platform/sqlite/migrate`; `agentstate` is not a target context or a new state hub. | Remove only after all active callers use context-owned repositories; migration 1–14 definitions/registration and upgrade/reopen fixtures pass; state-path and lock behavior have named owners; no production imports remain. |
| Legacy `internal/discovery` package | No production package remains in the PR-09.5c worktree: Inventory observation/reconciliation and Docker adapters moved in PR-09.5b; proposal types/repositories moved to Configuration in PR-09.5c. The `agentctl discovery proposals` command path is retained as a CLI compatibility surface, not as a Go package facade. | No compatibility package is retained. Confirm repository-wide imports and proposal/Inventory tests remain green before merge; preserve the CLI command name and output contract. |
| `internal/integration` shared types | Current Monitoring definitions and Configuration desired instances/templates share types in the same package. | Split owner types with explicit translation; keep aliases only for callers proven to require source compatibility. Remove aliases when repository-wide Go/import/CLI searches show zero consumers and persisted JSON/API representations remain unchanged. |
| `internal/coreapi` | Shared authenticated HTTP transport currently serves enrollment and Agent management calls. | Keep as a technical adapter until Identity, Configuration and Runtime consumers have explicit ports/DTO translation. Split only if independent client ownership reduces coupling; remove old package only at zero imports with protocol contract tests. |
| Former `internal/reconciliation` package; `internal/agentlifecycle` | Reconciliation controller and Configuration-owned workflow contracts moved to `internal/configuration/reconciliation`; attempt/evidence SQL persistence now lives in `internal/configuration/sqlite/reconciliation.go`. Runtime candidate/application contract lives in `internal/collectorruntime/contracts`; `internal/agentstate/sqlite_candidate_outcome.go` provides transaction-scoped Runtime journal evidence, and the remaining `sqlite_reconciliation_adapter.go` only translates recovery evidence. `agentlifecycle` continues to own Collector activation/readiness/recovery. | No legacy `internal/reconciliation` facade or reconciliation SQL facade remains. Retain the small adapter only while Runtime recovery state needs translation into the Configuration contract; remove it when composition can provide that translation directly without duplicating Runtime state models. Preserve same-transaction activation-journal checks, applied-generation checks, Configuration attempt updates and recovery evidence. Keep Collector lifecycle in Runtime.
| Legacy `internal/supervisor` path | Controlled rollback and explicit migration for existing installations require an alternate lifecycle owner during the supported migration window. | This is a policy-backed transitional runtime, not an indefinite facade. Retire only after all supported installations are explicitly migrated and the release/rollback policy closes the legacy path; never run both owners concurrently. |
| `internal/opampserver`, `internal/mockcore` | Separate protocol service and contract test infrastructure already have clear boundaries. | Retain; do not move under business contexts. Merge Mock Core test fixtures only if consumers and dependencies prove a net simplification. |

## Naming and documentation conventions

- Package names state a business context or concrete adapter (`identity`, `discovery/docker`, `operations/nats`), not a generic layer when only one implementation exists.
- Use names that reveal capability and owner: `EnrollmentService`, `ProviderSnapshot`, `DesiredSnapshot`, `CandidateApplier`, `Assessment`, `Executor`; do not create duplicate `Resource`, `Status`, or `Config` models with no translation owner.
- Exported cross-context types document invariants, lifecycle, concurrency, errors, side effects and caller obligations.
- Every package keeps a `doc.go` describing responsibility, main input/output, and explicit non-ownership, following the repository's Go documentation policy.
- Contract documents state version and whether they are Agent-local, approved shared, or proposed. Never silently publish local test contracts as cross-repository truth.
- Diagrams show process/system boundaries and ownership/control versus telemetry/data flow. Keep release-level behavior diagrams in existing architecture references; put source-migration views in this consolidation set.

## Structure acceptance criteria

1. No target directory is created without a source-identified cohesive capability and at least one real consumer/boundary.
2. No context imports another context's private persistence package.
3. Domain rules have no infrastructure imports; adapters depend inward through stable types/ports.
4. The single database and ordered schema migration sequence remain explicit; transaction owners remain reviewable.
5. `cmd/agentctl` retains public behavior but does not own new business policy or direct table SQL.
6. `internal/app` stays a composition/process-host boundary; its lifecycle orchestration is reduced only when characterization tests prove the new owner.
7. No change duplicates an OTel receiver, exporter, OpAMP protocol engine, event bus, SQLite repository framework, or generic plugin layer.
