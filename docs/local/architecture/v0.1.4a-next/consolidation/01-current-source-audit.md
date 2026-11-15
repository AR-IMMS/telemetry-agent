# PR-09.5 — Current Source Audit

**Status:** VERIFIED SOURCE SNAPSHOT, with review findings and proposed follow-ups separated below
**Baseline:** integration merge `b2599b8db31bfedfd75e360423d5aa98cfbcb5dc` (PR #32)
**Scope:** source tree, tests, local architecture/policy documents, and verification reports at that commit.

## Evidence boundary

The audited tree was read from the immutable merge commit above. The active workspace was on ancestor `feat/pr-07-opamp` at `3441c5a705c1d9577e2c1ee683622b562d040296`, with 111 modified or untracked paths, mostly source/tests. Those worktree changes were preserved and were not used as evidence for the baseline. The active workspace therefore is not a clean checkout of the requested integration branch; this planning set is anchored to the merge SHA, not to its dirty working tree.

`docs/shared` is a gitlink pinned to `c4cd59a9bcaaf0735c3107424f1be98fc498670a`. Its working directory and policy file were unavailable in the original audit checkout, so that audit recorded policy alignment as unverified. During the approved implementation kickoff, the exact pinned policy object was read from the submodule object database and compared with `docs/local/development/coding-policy.md`; no conflict was found. No shared content or gitlink was modified.

Labels used below:

- **Observed:** present in the baseline source or recorded verification evidence.
- **Finding:** an observed ownership or dependency condition that may complicate maintenance; not automatically a defect.
- **Approved target decision:** user-approved ownership rule for the planned consolidation; this does not claim the source already matches it.
- **Recommendation:** a proposed source migration detail, subject to approval of the complete target architecture and sequence.
- **Open:** remaining external policy or implementation-contract detail not resolved by these ownership decisions; do not guess during implementation.

## Executables and composition

| Baseline path / symbol | Observed responsibility | Architecture note |
|---|---|---|
| `cmd/agentctl/main.go`, `main`, root command dispatch | Single Go executable entrypoint found under `cmd/`; handles CLI/service command dispatch. | The repository does not have a separate `cmd/agent` directory at this baseline. Preserve the installed command surface and service startup contract. |
| `cmd/agentctl/composition.go`, `defaultDependencies` | Constructs platform, bootstrap, installation, Collector, dependency, identity, enrollment, health, discovery, and other adapters. | Composition belongs at the edge, but the large surface and the workflow-specific files adjacent to it make ownership difficult to see. Split only where a verified use case needs it. |
| `internal/app/runtime.go`, `RunRuntime` | Runs the service host, reconciliation loop, optional inventory/proposal/materialization loops, and optional Operations runner under a shared context. | This is more than passive dependency wiring. Decide which lifecycle coordination belongs to the host and which is Configuration/Runtime application behavior; retain cancellation and recovery semantics. |
| `internal/opampserver/server.go` | Implements the Go OpAMP server adapter using `opamp-go`; depends on Core API/configuration and identity types. | It is a control-plane protocol adapter in the test/Mock Core topology, not an eighth Agent business context. Keep its process/service boundary explicit. |
| `internal/mockcore/` | Contract-compatible Mock Core implementation used by OpAMP and integration verification. | Test infrastructure; keep production Core interoperability separately tracked. |

## Package inventory and source-grounded responsibility

The baseline contains these principal production packages. The target-owner column records approved logical ownership where a decision is listed below; it does not claim current files are already cleanly owned or prescribe a physical move.

| Existing package(s) | Observed symbols/files and behavior | Target owner / source finding |
|---|---|---|
| `internal/identity`, `internal/enrollment`, `internal/coreapi`, `internal/coreconfig`, `internal/credentialstore` | Platform identity collection; enrollment attempts/service; authenticated Core HTTP clients; Core endpoint/config values; protected secret storage. | **Identity.** Split OS/secure-store adapters from identity/enrollment rules only where interfaces are real. `identity` directly references Windows registry APIs; treat this as a platform adapter seam, not permission for domain code to gain more OS dependencies. |
| `internal/discovery`, `internal/discovery/docker`, `internal/discovery/sqlite` | Resource models/coordinator, provider snapshots, Docker client/normalization, SQLite inventory and proposal persistence. | **Inventory** owns observed resource evidence and provider snapshots. Proposal records currently reside here, but the approved target assigns proposal lifecycle, approval, and desired-state transition to Configuration. Preserve source identity and complete-snapshot semantics while transferring ownership. |
| `internal/integration`, `internal/integration/redis`, `internal/monitoringprofile` | Integration definitions, matching, template match rules, Redis endpoint behavior, profile registry/diagnostics and profile configuration. | **Monitoring** owns profile matching and recommendations, endpoint strategy, and monitoring intent. It queries Runtime's capability contract; it does not own or duplicate the pinned Collector catalog. Keep concrete profile additions independent of generic discovery. |
| `internal/collectorruntime` | Collector capability manifest/catalog, config compiler, candidate representation, validation. | **Runtime** owns the catalog derived from the pinned binary and candidate validation. Monitoring consumes a narrow capability-query contract (for example, whether `redisreceiver` is supported). Do not duplicate the catalog or split the manifest from the tested binary contract without a verified need. |
| `internal/config`, `internal/configuration`, `internal/configuration/sqlite`, `internal/reconciliation`, `internal/reconciliation/contracts` | YAML rendering/merge/readiness; desired Integration/Template state, approval and materialization services; SQLite repositories; desired-to-runtime convergence and candidate apply contracts. | **Configuration** owns desired state, templates, proposal lifecycle/approval, CAS and reconciliation decisions. `configuration` imports `discovery` and `integration`; `reconciliation` imports concrete `agentstate`, `collectorruntime`, `config`, `configuration`, and `integration`. Reduce concrete seams behind stable contracts only where a slice requires it. |
| `internal/agentinstallation`, `internal/agentlifecycle`, `internal/bootstrap`, `internal/dependency`, `internal/processowner`, `internal/supervisor`, `internal/opampsupervisorconfig` | Install/uninstall and Windows/systemd service adapters; bootstrap/artifact validation; Collector candidate activation/readiness/LKG; managed exporter/hardware dependencies; process ownership; legacy Collector process runner; OpAMP Supervisor config. | **Runtime.** `dependency` is comparatively broad (72 production Go files at baseline) and mixes catalog, artifact, configuration and platform lifecycle work. Decompose only along demonstrated capabilities; preserve exactly one Collector lifecycle owner. `internal/supervisor` and OpAMP Supervisor configuration are distinct concepts. |
| `internal/agenthealth` | Local snapshots, evidence assessment/reporting, Collector output and owner-specific observations. | **Health.** Evidence is read from its owner; Health must not take ownership of Inventory, Runtime or Operations persistence. |
| `internal/operations`, `internal/operations/handlers`, `internal/operations/dockeradapter`, `internal/operations/sqlite`, `internal/operations/nats` | Typed operation registry/executor; Diagnostics/Container/Dependency handlers; Docker inspect/restart adapter; SQLite execution/audit/outbox repository; authenticated JetStream transport/session runner. | **Operations.** This is already a cohesive capability boundary. It consumes evidence and narrow execution ports from Health, Inventory/Docker and Runtime/dependency owners. Preserve Core authorization ownership, local safety/audit, unknown-outcome behavior, credential revocation and monitoring independence. |
| `internal/agentstate` | `SQLiteStore`, state repository, schema initialization/migrations, identity/enrollment/runtime/inventory/configuration/Operations persistence, activation/reconciliation records, OS state path and lifecycle lock. | **Transitional shared persistence/migration seam**, not a business context. It imports many context types and child SQLite adapters. Target: one technical migration coordinator with table semantics/repositories owned by contexts; ordered definitions are registered at composition, preserving versions 1–14, order, and atomic transactions. |
| `internal/platform/sqlite`, `internal/platform/events` | SQLite open/durability settings and generic advisory event publisher. | **Shared technical infrastructure.** `platform/sqlite.Open` configures one connection, immediate transactions, WAL, `synchronous=FULL`, and a 5-second busy timeout; schema ownership is elsewhere. Events are post-commit wakeups, not authoritative state. |
| `internal/app`, `cmd/agentctl` | Service loop host and CLI parsing/formatting plus broad composition/workflow orchestration. | **Application Host and delivery edges**, not business contexts. App owns startup, shutdown, cancellation, and worker coordination; contexts own workflows and decisions. For example, App starts reconciliation while Configuration decides reconciliation and requests Runtime activation through an explicit contract. |
| `internal/opampserver`, `internal/mockcore`, `internal/operations/testsupport/mockcore` | OpAMP protocol server, Mock Core server, and broker binding/test fixtures. | **Central protocol/test infrastructure.** Keep server-side contract translation outside Agent domain packages; do not make test doubles production authorities. |

## Dependency observations

`go list` was run against an extracted copy of the baseline commit (no tests or builds were run). Relevant direct imports include:

- `cmd/agentctl` imports almost every context and adapter, including SQLite, Docker, Core, OpAMP-related configuration, Collector/runtime, and Operations packages.
- `internal/agentstate` imports `configuration`, `configuration/sqlite`, `discovery`, `discovery/sqlite`, `enrollment`, `enrollment/sqlite`, `identity`, `identity/sqlite`, `platform/events`, and `platform/sqlite`.
- `internal/configuration` imports `discovery` and `integration`; `internal/configuration/sqlite` imports the same domain types plus events.
- `internal/reconciliation` imports concrete `agentstate`, `collectorruntime`, `config`, `configuration`, `integration`, and its own contract package. `internal/reconciliation/contracts` itself imports `collectorruntime` and `config` types.
- `internal/agentlifecycle` coordinates `agenthealth`, `agentstate`, `bootstrap`, `config`, `coreconfig`, `credentialstore`, `dependency`, `enrollment`, `identity`, OpAMP Supervisor config, process ownership, reconciliation contracts, and the legacy supervisor.
- `internal/app` imports concrete Health, Runtime lifecycle, state, bootstrap, discovery, events, and reconciliation packages to start the Agent service loops.
- `internal/operations/handlers` consumes Health and Operations contracts; the Docker adapter consumes the Operations handler port; the transport and SQLite adapter depend on the Operations core types.
- `internal/opampserver` uses Core API/config and identity DTOs. Keep that translation edge explicit instead of making OpAMP protocol types cross business contexts.

These dependencies compile and are not in themselves evidence of a bug. The consolidation target is explicit dependency direction and discoverable ownership, not zero imports or a package-count metric.

## SQLite ownership, migration order, and invariants

**Observed migration coordinator:** `internal/agentstate/sqlite_store.go` calls `platform/sqlite.Open`, whose initialization callback calls `ensureSQLiteSchema` and imports legacy JSON state. `internal/agentstate/sqlite_schema.go` initializes `schema_migrations` and then runs migrations in fixed order:

| Version | Migration function / table family |
|---:|---|
| 1 | Base `agent_state` and activation journal schema |
| 2 | `installation_identity` |
| 3 | Core Gateway snapshot |
| 4 | Core Gateway runtime state |
| 5 | Enrollment attempts |
| 6 | Integration instances and desired-set revision |
| 7 | Candidate activation mapping/details |
| 8 | Candidate attempt history |
| 9 | Reconciliation diagnostics |
| 10 | Discovery inventory |
| 11 | Inventory write revision |
| 12 | Monitoring proposals |
| 13 | Integration templates |
| 14 | Operations executions/audit/result outbox |

`platform/sqlite.Open` owns connection policy, not domain DDL. `agentstate.SQLiteStore.Update` serializes in-process updates, uses an immediate SQLite transaction, checks the pending activation terminal-state invariant, writes state plus activation transition evidence, and commits before returning. Context-specific stores such as `configuration/sqlite`, `discovery/sqlite`, `identity/sqlite`, `enrollment/sqlite`, and `operations/sqlite` contain repository SQL, while the ordered migration entrypoint remains in `agentstate`.

Verified transaction families that must remain atomic include:

- Inventory provider snapshot, resource projection, completeness/write revision and change evidence in `discovery/sqlite/inventory.go`.
- Approval/desired state transition and revision checks in `configuration/sqlite/approval.go`.
- Template materialization with inventory/template/desired revision compare-and-swap in `configuration/sqlite/materialization.go`.
- Operations execution claim/completion, audit rows and result-outbox insertion in `operations/sqlite/repository.go`.
- Enrollment attempt transitions and installation binding changes in `enrollment/sqlite/repository.go`.
- Runtime activation state and activation journal updates in `agentstate`.

The exact owning transaction and failure/retry guarantees must be rechecked per operation before moving repository code. Do not split a write across context stores and replace atomicity with events.

## Compatibility surface and verification

**Observed preserved surface:** `agentctl` CLI commands/flags and service lifecycle; enrollment/binding and protected credential behavior; SQLite state/migrations; Docker inventory and proposal history; Integration and Template revisions/materialization; no-op reconciliation; Collector validation, activation, LKG and recovery; managed dependencies; Monitoring profiles and OTel receiver ownership; OpAMP Supervisor lifecycle policy; Operations request authentication, durable idempotency/audit/outbox and restart safety. PR-09 verification is in [its report](../../../verification/v0.1.4a-next/pr-verification/pr-09-report.md) in the merged baseline.

Tests are primarily colocated `_test.go` files, with Docker/Collector/OTLP and Agent-managed Supervisor E2E tests in `cmd/agentctl` and `internal/agentinstallation`/`internal/agentlifecycle`. Contract fixtures and test infrastructure also live in `internal/mockcore` and `internal/operations/testsupport`. Packaging and scripts remain repository-level platform/deployment concerns; do not move them into business contexts solely for symmetry.

### Entry-point, CLI, contracts and repository-level assets

- **Entrypoint:** `cmd/agentctl/main.go:main` is the only tracked `main` found under `cmd/` at the audited commit. Its dispatcher routes `bootstrap`, `dependency`, `inventory`, `discovery`, `integration`, `enroll`, `service`, `install`, and `uninstall`; command-specific files also cover Collector/status, templates, proposals and Operations. The route list is verified in `cmd/agentctl/main.go`; it is not a recommendation to remove commands.
- **Workflow evidence:** `cmd/agentctl/reconciliation_runtime.go` composes the runtime reconciliation controller; `proposal_approval_runtime.go` wires proposal approval; `template_materialization_runtime.go` runs template projection. CLI tests cover persisted Inventory snapshots, explicit proposal approval, template CAS/output compatibility, service lifecycle, and shutdown. These are candidate workflow seams, not proof that CLI is the correct owner.
- **Agent-local contracts:** `contracts/operations/operation-request-v1.schema.json`, `operation-result-v1.schema.json`, and `broker-binding-v1.schema.json`; OpAMP/Core control specification at `docs/local/architecture/v0.1.4a-next/contracts/agent-control-v1.openapi.yaml`. The Agent-local Operations v1 contracts and Mock Core pass implementation gates; they do not assert production NestJS interoperability.
- **Infrastructure/config:** `configs/base`, `configs/os`, `configs/profiles`, and `configs/monitoring-profiles` hold Collector and profile inputs; `infra/observability/compose.yaml` and gateway/Prometheus/Grafana fixtures support live E2E. Treat these as deploy/test configuration boundaries, not business models.
- **Quality policy:** `scripts/check-architecture.ps1` enforces the approved import/file-size checks at this baseline. `docs/local/development/coding-policy.md` is a supplement and defers conflicts to `docs/shared/engineering/AI_CODING_POLICY.md`; the exact pinned policy was subsequently verified during implementation kickoff as recorded above.

### Documentation contradictions reconciled by this set

- The old `source-structure.md` contains pre-audit package-to-context guesses. They are retained as history but superseded for current placement by this audit and [04-source-structure](04-source-structure.md).
- `architecture-overview.md` and `opamp-integration.md` were written before all PR-07/08/09 closeout evidence existed. Their capability goals remain useful; current implementation statuses come from the release tracker and checkpoint reports. The architecture index now points readers to that precedence.
- `state-and-event-model.md`'s “no external broker/outbox in MVP” describes the internal advisory event dispatcher. It does not describe PR-09's separate authenticated JetStream transport and durable result outbox.
- `context-boundaries.md` is a logical ownership design baseline, not a source placement audit. The seven current context names remain aligned; package assignments are corrected in [02-context-ownership](02-context-ownership.md).

## Approved target decisions and remaining review points

1. Keep the seven-context logical model; describe Application Host, protocol adapters, persistence/event infrastructure, test fixtures and the Collector as supporting boundaries.
2. Establish context contracts from existing exported types/use cases before creating new package layers. Keep the current package tree as a flexible migration baseline.
3. Treat `cmd/agentctl`, `agentstate`, `agentlifecycle`, `configuration`, `reconciliation`, `dependency`, and the `collectorruntime`/Monitoring seam as highest-value audit targets.
4. **Approved: SQLite migration coordination.** Use one centralized technical coordinator. Contexts own table semantics and repositories; ordered definitions are registered at the composition boundary. The platform package must not import business contexts. Preserve migrations 1–14, exact order, and existing atomic transactions.
5. **Approved: proposal ownership.** Inventory owns observations; Monitoring owns profile matching/recommendations; Configuration owns proposal creation/lifecycle, approval, and the desired-state transition. The proposal and desired-state update remain atomic.
6. **Approved: capability catalog ownership.** Runtime owns the catalog derived from the pinned Collector binary; Monitoring consumes a narrow query contract. No duplicated catalog.
7. **Approved: managed dependencies.** Runtime owns definitions, provisioning, and lifecycle. Organize cohesive capabilities where source and consumers justify it; do not mechanically move the oversized `dependency` package.
8. **Approved: host/workflow boundary.** Application Host owns process and worker coordination. Business contexts own workflow decisions; Configuration decides reconciliation and requests Runtime activation explicitly.
9. **Approved: Core configuration authority.** Preserve the already-approved Core/local authority and revision semantics. Track unresolved control-plane policy separately; structural consolidation does not resolve it.
10. Preserve a compatibility facade while public/CLI consumers exist. Remove it only after repository-wide and test consumer checks plus rollback review.
11. Do not combine structural moves with changes to protocols, SQLite format, CLI behavior, receiver ownership, Supervisor default, security policy, or runtime behavior.

### Remaining review points

- Define the future proposal-to-Configuration contract and its transaction boundary from existing `configuration/sqlite/approval.go`, `discovery/sqlite` proposal storage, and migration 12. Ownership is decided; exact types/adapters and a no-data-loss transfer are not.
- The pinned shared coding policy was verified against the local supplement during implementation kickoff; preserve the gitlink and do not edit `docs/shared/` in this work.
- Keep unresolved Core/security/control-plane policy questions in their existing decision record; do not expand this refactoring plan to decide them.

The full approved owner matrix and target dependency contracts are in [02-context-ownership.md](02-context-ownership.md); migration gates are in [the PR-09.5 plan](../../../plans/v0.1.4a-next/pr-09.5-architecture-consolidation.md). The complete target and package-import diagrams are in [03-target-architecture.md](03-target-architecture.md).
