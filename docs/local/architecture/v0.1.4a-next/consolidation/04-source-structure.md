# PR-09.5 — Flexible Source Structure

**Status:** concrete, source-grounded target for review; package names remain flexible and the tree is not a required scaffold.
**Evidence:** [source audit](01-current-source-audit.md) at `b2599b8db31bfedfd75e360423d5aa98cfbcb5dc`; ownership decisions in [02-context-ownership](02-context-ownership.md).

**Evidence boundary:** the active worktree is an older, dirty ancestor and is not the source of truth for the target map. This disposition uses the immutable PR-09 integration tree recorded by the source audit and its path/import/transaction inventory. The available code-graph index is older than that integration tree, so its community scores are exploratory only and were not used to assign owners.

## Structure principles

- Organize by business context first, cohesive capability second, and technical layer only where the code needs it.
- Keep a small context flat. Introduce `domain`, `application`, `sqlite`, or `platform` subpackages only to make a real dependency boundary or capability easier to test and own.
- Keep a capability's cohesive types, use case, and tests discoverable; do not move a file because its name resembles a target folder.
- The owning context defines invariants and transaction semantics. A shared adapter may provide SQLite handles and migration ordering without owning every table.
- Keep public contracts explicit and small. Avoid generic `common`, `utils`, `services`, or `models` buckets, generated-empty layers, service locators, reflection DI, and a custom plugin framework.
- Prefer characterization tests and short-lived compatibility facades over a big-bang move. Every facade must name its active consumers, reason to retain it, and measurable removal criteria; a facade without those is not part of the target.

## Concrete target tree

This is the proposed physical navigation after migration, grounded in the source snapshot above. It is not a mandatory package scaffold: keep a capability flat where its types and use cases remain cohesive, and introduce a technical layer only for an actual adapter, transaction, or platform boundary. Parenthetical source paths are the current baseline locations, not proposed permanent duplicate packages.

```text
cmd/agentctl/                                  CLI and service delivery edge
internal/
├── app/                                       composition, process host, worker lifecycle
├── identity/
│   ├── host/                                  machine and platform facts
│   ├── installation/                          stable installation ID and binding persistence
│   ├── enrollment/                            enrollment attempt and retry use cases
│   └── credentials/                           protected credential store adapters
├── inventory/
│   ├── observation/                           provider coordination, normalization, completeness
│   ├── docker/                                Docker API provider
│   └── sqlite/                                inventory-owned rows and snapshots
├── monitoring/
│   ├── profiles/                              profile registry, Redis definition, config loading
│   ├── matching/                              selectors, recommendation and endpoint diagnostics
│   └── templates/                             monitoring-side template match evaluation only
├── configuration/
│   ├── desiredstate/                          Integration desired instances and revisions
│   ├── remoteconfig/                           Core snapshot, authority and revision semantics
│   ├── proposals/                             proposal lifecycle and approval policy
│   ├── templates/                             template definitions, persistence and ownership
│   ├── materialization/                       CAS plan and atomic desired-state projection
│   ├── reconciliation/                        desired-to-effective decisions and apply requests
│   └── rendering/                              deterministic layered configuration rendering
├── runtime/
│   ├── bootstrap/                             first-run setup and recovery entrypoints
│   ├── artifacts/                             pin, download, validate and publish binaries/config
│   ├── dependencies/
│   │   ├── catalog/                            managed dependency definitions
│   │   ├── nodeexporter/                       Linux dependency capability
│   │   ├── windowsexporter/                    Windows dependency capability
│   │   ├── librehardwaremonitor/                Windows hardware dependency capability
│   │   └── adapters/                           shared OS/service/process seams only if justified
│   ├── collector/
│   │   ├── capabilities/                       catalog derived from pinned Collector binary
│   │   ├── configuration/                      compile and validate candidate config
│   │   └── activation/                          single-owner activation, readiness, LKG/recovery
│   ├── opamp/                                  managed Supervisor provisioning/config/status adapter
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
├── adapters/
│   └── coreapi/                                Core HTTP protocol translation shared by owners
├── platform/
│   ├── sqlite/                                 connection policy
│   │   └── migrate/                            generic ordered migration executor
│   └── events/                                 bounded post-commit advisory notifications
├── opampserver/                                separate Go OpAMP server/service boundary
└── mockcore/                                   replaceable contract test service, test-only authority

configs/                                        Collector/profile inputs, not generated state
contracts/                                     versioned Agent wire schemas; keep shared contracts in docs/shared
infra/                                          isolated Docker/OTLP testbeds
docs/                                           architecture, contracts, plans, runbooks and evidence
```

`internal/opampserver` is a Core-facing protocol service and remains outside Agent business contexts. `internal/mockcore` is a contract test service, not a policy owner. `internal/operations/testsupport/mockcore` remains a narrow Operations/NATS fixture unless its actual consumers can use the full Mock Core without adding production dependencies. Neither is nested under Runtime's `opamp/`, which is only the Agent's managed OpAMP Supervisor integration.

There is no target `internal/state` or generic state hub. The current `internal/agentstate` is a transitional multi-context persistence adapter. Context repositories own their tables and transaction semantics. One technical migration coordinator executes ordered migration definitions registered by the composition root; the platform coordinator imports no business context. Preserve migration versions 1–14, order, and every existing atomic transaction. Retire `agentstate` only when all active consumers use owner repositories, migration-upgrade/reopen fixtures pass, and no state/path/process-lock behavior remains stranded there.

### Package disposition matrix by context and capability

Each row names a responsibility, source candidates, intended boundary, dependencies and invariants. Movement is incremental: split mixed files/models by semantic owner only after callers and translations are characterized. A package may remain at its current path if moving it does not improve ownership.

#### Identity

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Host identity: machine IDs, OS/platform facts, release metadata | `identity/machine*.go`, `identity/platform*.go`, `identity/os_release.go` → `identity/host` | OS adapters only; stable identity inputs, Windows/Linux behavior, and platform-specific file isolation. |
| Installation identity: parse/create installation ID and bind durable installation record | `identity/installation_id.go`, `installation_identity.go`, `identity/sqlite/repository.go`, `agentstate/sqlite_identity.go` → `identity/installation` plus `sqlite` adapter | Host identity and SQLite port; stable installation ID and atomic initialize/read semantics. |
| Enrollment and binding: attempt lifecycle, uncertain outcomes, retry/resume | `enrollment/{model,service}.go`, `enrollment/sqlite/repository.go`, `agentstate/sqlite_enrollment.go` and `sqlite_enrollment_schema.go` → `identity/enrollment` | Installation identity, Core enrollment port, protected credential port; preserve persisted uncertainty, retry classification and existing Core response semantics. |
| Credential protection: store, replace and revoke secret material | `credentialstore/store*.go` → `identity/credentials` | OS ACL/DPAPI or Linux file permissions; secrets never enter logs, SQLite rows, CLI output or diagnostics. |

`coreapi` is currently a narrow shared HTTP adapter, not all Identity business logic. Keep its transport boundary explicit; split enrollment requests from control/configuration endpoints only after the consumed contracts and DTO translation are mapped. `coreconfig.GatewaySnapshot` authority/revision semantics belong to Configuration's `remoteconfig`, not host Identity.

#### Inventory

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Observation coordination and normalization | `discovery/{coordinator,types,normalize,reconcile}.go` → `inventory/observation` | Provider contract, Identity association and Monitoring read port; complete snapshots only, provider-scoped IDs, provenance, missing/history semantics. `reconcile.go` resource absence logic stays here; proposal projection does not. |
| Docker discovery | `discovery/docker/provider.go` → `inventory/docker` | Docker API plus observation contract; labels/endpoints/network metadata remain evidence, incomplete or failed scans never delete inventory. |
| Inventory persistence | `discovery/sqlite/{store,inventory}.go` and `agentstate/sqlite_discovery.go`, `sqlite_inventory_schema.go` → `inventory/sqlite` | Central migration registration; atomic snapshot, completeness, write revision and change evidence. Preserve migrations 10–11. |

#### Monitoring

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Profile definitions and resolution | `integration/{integration,registry,resolver}.go`, `integration/redis/definition.go`, `monitoringprofile/{profile,loader,docker_config}.go` → `monitoring/profiles` | Inventory observation port and Runtime capability-query contract; exact definition version, profile extensibility, no generic-discovery edits for new profiles. |
| Resource matching and recommendation | `integration/{matching,matching_engine}.go`, `monitoringprofile/diagnostics.go`, and `integration/template_matching.go` match evaluator → `monitoring/matching` and `monitoring/templates` | Inventory projection, profile registry and Runtime catalog query; explicit match/required-capability/endpoint outcomes, no receiver lifecycle side effect. |
| Receiver capability query | Current source `collectorruntime/{capability,manifest}.go` → Runtime-owned `runtime/collector/capabilities`; Monitoring consumes only a narrow query port | Pinned Collector binary/manifest is the sole catalog source. Query (including `redisreceiver`) before resolving a profile; never copy the catalog into Monitoring. |

Template matching is a Monitoring computation over Configuration-owned template definitions. `integration/template.go` currently mixes template policy/binding models with integration types; split owner-specific models and translate through the reviewed contract, rather than moving the whole file or sharing an unowned `integration` model.

#### Configuration

| Capability and responsibility | Current source → intended package | Dependencies / transaction invariants to preserve |
|---|---|---|
| Desired state: accepted Integration instances and revisioned reads/writes | `configuration/{integration.go,sqlite/integration.go}`, `integration.IntegrationInstance`, `agentstate/sqlite_configuration.go` and integration schema → `configuration/desiredstate` | Monitoring's resolved intent, SQLite repository; expected revision checked in the same write, stable IDs, no-op revisions remain no-op. Split shared `integration` structs by owner with explicit translation. |
| Remote configuration: authenticated Core Gateway snapshot, accepted revision and effective authority | `coreconfig/gateway.go`, `agentstate/sqlite_core_gateway*.go`, Core control DTO use in `coreapi/control.go` → `configuration/remoteconfig` plus Core API adapter | Core API authentication and local/bootstrap input; preserve approved Core/local authority and revision rules. Do not decide still-open policy such as namespace relinquishment here. |
| Proposals: observed candidate lifecycle, validation, dismissal and approval decision | `discovery/{proposal,proposal_validation}.go`, `discovery/sqlite/proposal.go`, `configuration/approval.go`, `configuration/sqlite/approval.go`, `agentstate/sqlite_proposal_schema.go` → `configuration/proposals` | Inventory evidence and Monitoring recommendation; proposal is Configuration-owned. Preserve migration 12, proposal identity/revisions/history, and one transaction for proposal creation/approval plus desired-state transition. `discovery/reconcile.go` remains Inventory resource-missing reconciliation. |
| Templates: reusable configuration definitions, authority, revisions and persistence | `integration/template.go` models, `configuration/template.go`, `configuration/sqlite/template.go`, `agentstate/sqlite_template_schema.go` → `configuration/templates` | Monitoring match results, desiredstate and Inventory revision port; preserve owner, tombstone/ID reuse rules and deterministic setting resolution. Matching itself stays in Monitoring. |
| Materialization: plan template bindings into desired instances | `configuration/materialization.go`, `configuration/sqlite/materialization.go` → `configuration/materialization` | Templates, Inventory snapshot/revision and desired state; preserve one CAS over Inventory, template and desired revisions, stable generated IDs, and atomic batch writes. |
| Reconciliation: decide desired-to-effective convergence and request apply | `reconciliation/{controller,ports,sqlite}.go`, `reconciliation/contracts/ports.go`, `agentstate/sqlite_reconciliation*.go` → `configuration/reconciliation`; Runtime owns apply implementation | Desired revision, compiler/validator and Runtime apply-result contract; preserve periodic/startup recovery, bounded retry and status/revision attribution. `internal/app` starts/stops worker; it does not decide. |
| Rendering: merge deterministic base/profile/OS/vendor/local layers and write candidate input | `config/{types,merge,renderer,writer,validator,readiness,metrics_pipeline}.go` → `configuration/rendering` if owner-boundary tests confirm; keep the focused `internal/config` package if moving adds no seam | Configuration layers, Identity values, Monitoring intent; deterministic merge, validation before activation, explicit provenance and no arbitrary remote YAML. Runtime's Collector compiler/capability validation remains separate. |

Configuration is intentionally split by lifecycle, not one directory of generic services. The current `integration` and `discovery` packages mix policy with profile/evidence models; do not move a file wholesale when it contains multiple owners. The proposal/desired-state transaction is the strongest cross-package atomic boundary and must remain one SQLite transaction after extraction.

#### Runtime

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Bootstrap: first install, configuration preparation and recovery entrypoints | `bootstrap/{bootstrap,install,command,config_activation,config_recovery}.go` → `runtime/bootstrap` | CLI/application host and artifact/service ports; idempotent install, no change to command behavior or operator config ownership. |
| Artifact management: pinned download, checksum/verification, staging and publication | `bootstrap/{artifact,download,publish*}.go`, installation binary copy → `runtime/artifacts` | Versioned manifests and OS filesystem/service adapters; checksum and atomic replacement remain mandatory. Split `supervisor_artifact.go` only if its artifact and OpAMP configuration responsibilities cannot stay cohesive. |
| Managed dependency catalog | `dependency/{catalog,types}.go` → `runtime/dependencies/catalog` | Static supported dependency definitions; one canonical catalog, stable CLI names and no speculative plugin loading. |
| Dependency provisioning/lifecycle: Node Exporter | `dependency/node_exporter/*.go` → `runtime/dependencies/nodeexporter` with platform files retained inside the capability unless a shared adapter seam is proven | Catalog, artifact ports, systemd/process adapters; preserve managed ownership, idempotent install/enable/reconcile/teardown, host privilege and no removal of unowned installs. |
| Dependency provisioning/lifecycle: Windows Exporter | `dependency/windows_exporter_*.go` → `runtime/dependencies/windowsexporter` | Catalog, MSI/artifact and Windows SCM adapters; preserve preflight/admin checks, ownership markers, service recovery and safe uninstall. |
| Dependency provisioning/lifecycle: LibreHardwareMonitor | `dependency/libre_hardware_monitor/*.go` → `runtime/dependencies/librehardwaremonitor` | Catalog, artifact, Windows task/firewall/process adapters; preserve secret-free config, firewall/task ownership, health checks and cleanup. |
| Shared dependency platform primitives | `dependency/{service,process}.go` and any truly shared helpers → `runtime/dependencies/adapters` only after importer audit | Capability packages; platform-neutral contracts and Windows/Linux build constraints. Do not force exporter-specific lifecycles into a generic lifecycle framework. |
| Collector catalog, compile and validate | `collectorruntime/{capability,manifest,compiler,validation}.go` → `runtime/collector/{capabilities,configuration}` | Monitoring query, Configuration rendered input, Identity platform; pinned manifest is canonical and candidate validation occurs before activation. |
| Collector activation, readiness, rollback and recovery | `agentlifecycle/{candidate*,configuration,rollback,activation_recovery,runtime*,teardown*,baseline}.go`, `supervisor/*.go` → `runtime/collector/activation` plus `runtime/process` where process ownership is distinct | Configuration apply request, Collector adapter and durable activation journal; one lifecycle owner, generation evidence, readiness, LKG and terminal-state guard. Preserve controlled legacy fallback during OpAMP Supervisor migration. |
| Managed OpAMP Supervisor provisioning and status | `opampsupervisorconfig/config.go`, `agentlifecycle/supervisor_runtime.go`, `bootstrap/supervisor_artifact.go` → `runtime/opamp` | Runtime artifact, Collector lifecycle and Core control adapter; Supervisor is a managed dependency/protocol participant, not the OpAMP Server and never a second simultaneous Collector owner. |
| Agent service installation and native lifecycle | `agentinstallation/{installer,layout,service_default*,windows_service*,systemd*,remover*,uninstall_guard}.go` → `runtime/service` | Bootstrap/artifact and platform service managers; preserve installation layout, state, Windows Service/systemd behavior, explicit migration and uninstall ownership. |
| Process identity and cleanup | `processowner/*`, `supervisor/{child,child_unix,child_windows}.go`, process portions of `agentlifecycle` → `runtime/process` | Native process APIs and lifecycle coordinator; bounded shutdown, process-tree cleanup and no duplicate process owner. |

The existing `internal/dependency` has 72 production Go files, but the source already identifies three substantial capabilities. Decompose by those capabilities first; introduce shared `catalog`, `provisioning`, `artifacts`, `lifecycle`, or `adapters` subpackages only where imports and duplicated behavior prove cohesion. Do not create empty package layers to match the illustrative tree.

#### Health and Operations

| Capability and responsibility | Current source → intended package | Dependencies / invariants to preserve |
|---|---|---|
| Health snapshot and evidence model | `agenthealth/{snapshot,assessment,owner_evidence,sanitize}.go` → `health/{snapshot,evidence}` | Narrow owner evidence ports from Identity, Inventory, Runtime and Collector diagnostics; timestamps/freshness/redaction; missing evidence is UNKNOWN, readiness is not delivery. |
| Health transports/reporting | `agenthealth/{client,http,server,reporter}.go` → `health/reporting` | Existing local status and approved reporting contracts only; no custom Gateway probe or invented Core endpoint/schema. |
| Operations command/registry/executor | `operations/{operation,executor}.go` → `operations/command`; wire schemas stay in `contracts/operations/*.json` | Core-authorized request contract and handler ports; exact-target validation, idempotency, duplicate prevention and durable UNKNOWN outcomes. |
| Operations handlers and Docker adapter | `operations/handlers/*.go`, `operations/dockeradapter/runtime.go` → `operations/handlers` and `operations/dockeradapter` | Health, Inventory and Runtime narrow ports; diagnostics/inspect/restart only; local opt-in, exact immutable container ID and post-restart verification remain. |
| Operations transport and persistence | `operations/nats/transport.go`, `operations/sqlite/repository.go`, `agentstate/sqlite_operations_schema.go` → existing `operations/nats` and `operations/sqlite` adapters | Core-provisioned binding; preserve credential invalidation, monitor independence, one transaction for execution/audit/outbox, replay and unknown-outcome rules. |

#### Supporting technical and protocol boundaries

| Boundary | Current source → intended boundary | Dependencies / invariants to preserve |
|---|---|---|
| Composition and worker host | `app/runtime.go`, `cmd/agentctl/composition.go` and command wiring → `internal/app` / `cmd/agentctl` | App starts/stops workers and composes context ports; contexts decide business workflows; CLI flags/output/exit codes remain compatible. |
| SQLite connection and migration coordination | `platform/sqlite/sqlite.go`, `agentstate/sqlite_schema.go`, context `*/sqlite` repositories → `platform/sqlite` and its technical `migrate` subpackage, plus context-owned ordered definitions registered by App | One database; one coordinator; schema versions 1–14 and order unchanged; no context imports by platform; per-operation atomic transactions unchanged. No replacement `state` package. |
| Advisory events | `platform/events/dispatcher.go` → `platform/events` | Post-commit wakeups only; SQLite and startup/periodic recovery remain authoritative. |
| Core HTTP protocol adapter | `coreapi/{enrollment_client,control}.go` → retain `internal/coreapi` or move to `internal/adapters/coreapi` after consumer split | Identity enrollment, Configuration remoteconfig and Runtime status call sites; keep transport DTOs at edge and do not merge client protocol with business policy. |
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
| `consolidation/01-current-source-audit.md` | Source facts at the recorded immutable merge, import observations, migrations and atomicity. | Canonical current-source audit. Do not duplicate package inventories in other architecture docs. Refresh at each implementation baseline. |
| `consolidation/02-context-ownership.md` | Approved OD-01–OD-06 logical ownership and collaboration flow. | Canonical context owner map. Keep package destinations in this document only as short links, not a second tree. |
| `consolidation/03-target-architecture.md` | Target context contracts, collaboration flow and separate Go-import direction. | Canonical interaction/dependency diagrams. The context-boundaries document remains a compact logical summary. |
| `consolidation/04-source-structure.md` | Capability tree, source-to-target disposition, boundaries, retention/removal criteria and documentation audit. | Canonical physical target approved 2026-11-15; update the migration plan from this file rather than maintaining duplicate move maps. |
| `plans/v0.1.4a-next/pr-09.5-architecture-consolidation.md` | Ordered implementation slices, gates, characterization, verification and rollback. | Canonical migration execution plan; avoid repeating full ownership tables here. |
| `verification/v0.1.4a-next/pr-verification/` | Evidence and checkpoint outcomes. | Canonical claims about what is implemented/tested; plans and architecture diagrams are not verification evidence. |
| `context-boundaries.md`, `source-structure.md`, `state-and-event-model.md`, `opamp-integration.md` | Logical context summary, historical package proposal, persistence invariants and OpAMP runtime/contracts. | Keep targeted, link to the consolidation set for current placement. `source-structure.md` is already marked historical and superseded; retain it for design history, not as migration input. |
| `consolidation/README.md` and `architecture/v0.1.4a-next/README.md` | Review-set and release navigation. | The former indexes the five planning artifacts; the latter indexes all release docs. Avoid copying their explanations into each other. |

**Archival opportunity:** after approval, move no records as part of source migration. A later documentation-only cleanup may move superseded, unversioned package trees/checklists to `docs/local/archive/` or leave them in place with the existing supersession banner. First verify inbound links, dated decision provenance and policy-history needs; preserve links or add a short archival index. Do not archive release contracts, verification evidence, runbooks, or historical decision records that explain a still-supported migration path.

### Compatibility and package-retention decisions

| Existing package or facade | Retention reason during migration | Removal / final-location criteria |
|---|---|---|
| `internal/agentstate` | Current SQLite initialization, legacy state import, activation journal, cross-process locks and migrations are coupled here. It is not a target context or a new state hub. | Remove only after all active callers use context-owned repositories; migration 1–14 registration and upgrade/reopen fixtures pass; state-path and lock behavior have named owners; no production imports remain. |
| `internal/discovery` proposal symbols and `discovery/sqlite/proposal` | Existing CLI, proposal history and approval code still use these types/repository contracts. | Retain only as a forwarding adapter while Configuration owns migration-12 reads/writes and atomic approval. Remove after all callers use Configuration contracts and CLI/data fixtures pass. |
| `internal/integration` shared types | Current Monitoring definitions and Configuration desired instances/templates share types in the same package. | Split owner types with explicit translation; keep aliases only for callers proven to require source compatibility. Remove aliases when repository-wide Go/import/CLI searches show zero consumers and persisted JSON/API representations remain unchanged. |
| `internal/coreapi` | Shared authenticated HTTP transport currently serves enrollment and Agent management calls. | Keep as a technical adapter until Identity, Configuration and Runtime consumers have explicit ports/DTO translation. Split only if independent client ownership reduces coupling; remove old package only at zero imports with protocol contract tests. |
| `internal/reconciliation` and `internal/agentlifecycle` | Cross-context controllers currently bridge desired state, runtime candidate validation and lifecycle recovery. | Move one use case at a time to Configuration decisions and Runtime apply ports. Remove compatibility wrappers after call graph/import checks, characterization tests and restart/LKG E2E pass. |
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
