# PR-04 — Bounded Context Ownership & Repository Migration

**Status:** PR-04a identity and PR-04b enrollment adapters PASS and merged; overall PR-04 remains IN_PROGRESS. **Release target:** `v0.1.4a-next`. **PR-04a merge:** `4bdfff72ea1bcc6b344ced4fdc65f5d0b3df3301`; **PR-04b merge:** `bdc656acc8f6c6cfa3fb09f8cf4cc3d3a8e8e9cc`.

## Objective and exit boundary

Align package ownership with seven contexts (six established plus deferred Operations) while preserving SQLite schema, public CLI and all runtime behavior. This is a source-refactoring PR, not a new feature.

## Baseline / current architecture constraints

The current source confirms a broad persistence and orchestration seam; the ownership matrix below is a review input, not proof every proposed move is already safe.

### Verified source baseline

- `internal/agentstate` owns `SQLiteStore.open`, schema setup and the ordered schema migrations. `SQLiteStore` opens one operational database and initializes all registered context tables on access. `NewIdentityStore` and `NewEnrollmentStore` are compatibility constructors over that same store.
- Identity SQL implements `identity.InstallationIdentityRepository`; initialization is a transaction that validates and stores one UUID, then returns the committed value. Enrollment SQL implements `enrollment.AttemptRepository`; SQLite contains request/result metadata and protected-store references, while credential values are passed to the credential-store adapter, not persisted here.
- Enrollment operations have observable recovery semantics: a lost response remains a durable pending attempt; explicit re-enrollment retains prior binding metadata and credential references; retired credential references are cleared only after protected-store deletion succeeds. `CompleteAttempt` commits the validated Core result and protected credential reference atomically.
- `internal/agentstate/sqlite_approval.go` owns the cross-table approval transaction. It validates proposal and inventory revisions, desired-state CAS, uniqueness, then writes desired Integration state and the approved proposal together. `internal/agentstate/sqlite_materialization.go` similarly applies revision-bound Template projections transactionally. Moving either half independently would break atomicity.
- `internal/reconciliation` imports `internal/agentlifecycle` for the clean-baseline and candidate-apply DTOs, and imports `internal/agentstate` for durable attempt/error and candidate-outcome types. `agentlifecycle.ApplyCollectorCandidate` validates generation/fingerprint/digest and delegates activation through the lifecycle coordinator. Runtime activation journaling and Agent-state updates are coupled.
- The supported workflow uses the Windows worktree branch `feat/v0.1.4a-next-work`. At the start of this slice, the only worktree artifact was the preserved untracked zero-byte `cmd/agentctl/otel.yaml.lifecycle.lock`; the current implementation diff is limited to the identity adapter, its compatibility seam/test, and this plan/progress record. `docs/shared/` remains untouched. A submodule status command could not read Git's Windows named-pipe/session object in this shell; parent status with submodules ignored shows no shared gitlink modification.

### Contracts required before a migration slice

| Slice | Contract that must remain stable | Current owner / evidence | Migration constraint |
|---|---|---|---|
| Identity persistence | Valid UUID input; first initialization wins; concurrent/repeated calls return the persisted UUID; malformed stored state fails visibly; SQLite schema/version remains compatible. | `identity.InstallationIdentityRepository`; `agentstate.EnsureInstallationID`; `installation_identity` schema migration. | Keep `NewIdentityStore` and old-database behavior until all callers and fixtures use the replacement; no UUID regeneration or schema rewrite. |
| Enrollment persistence | Immutable pending request; no secret values in SQLite; protected credential references only; atomic binding/result completion; explicit failure/re-enrollment rules and credential cleanup ordering. | `enrollment.AttemptRepository`; `agentstate.sqlite_enrollment`; `credentialstore`; `coreapi` caller. | Preserve old lost-response/revocation behavior and secret-store side effects. A persistence refactor cannot change Core/security policy. |
| Proposal approval | Validate proposal revision, inventory write revision, desired revision and resource binding; approval plus desired Integration update is one transaction; idempotent retry returns the committed instance. | `configuration.ApprovalRepository`; `agentstate.sqlite_approval`. | Keep this use case and transaction together even if persistence files/packages move. |
| Template projection | Validate complete revision tokens; apply upserts/suspensions and desired revision atomically; queue wakeup only after commit. | `configuration.MaterializationRepository`; `agentstate.sqlite_materialization`. | Do not split repository operations across stores or change stale/missing semantics. |
| Candidate application | Candidate bytes/digest/platform, baseline fingerprint and expected generation are validated; stale inputs fail before activation; activation/LKG journal remains coordinated. | `agentlifecycle.ApplyCollectorCandidate`, `CandidateApplyRequest`, `CandidateActivationOutcome`; `reconciliation.Controller`. | A new port/DTO owner must be selected before removing the existing import. Do not add duplicate DTOs or a generic common package. |

## Contract audit ruling and migration order

1. **Ruling:** keep schema migration and database opening under `agentstate` for this slice, and pass its opener into the context-owned identity SQLite adapter. This preserves the single migration order and old database path without adding a generic provider package. Cost if wrong: `agentstate` remains a shared technical infrastructure owner and may need a later provider extraction if other contexts require independent storage composition.
2. Keep approval and Template materialization out of the first adapter extraction: each is a multi-table transaction spanning different domain records and must move as one repository operation.
3. **Ruling:** do not invert the `reconciliation -> agentlifecycle` import by inventing a new shared DTO package. `agentlifecycle` currently supplies the runtime-owned baseline and candidate application contract; the source dependency is therefore not by itself proof of a boundary violation. Before changing it, decide whether the target requires a consumer-owned port with an explicit adapter, or whether the runtime-owned port is the accepted contract. Either choice must have one canonical request/result DTO and retain generation, fingerprint, digest, readiness, and LKG semantics.
4. **Ruling:** inject the existing `agentstate` SQLite write mutex into the enrollment adapter rather than creating a second lock. This keeps in-process writes serialized across repository values while SQLite immediate transactions and busy timeout continue coordinating other processes. Cost if wrong: sharing one mutex can increase local writer contention, but replacing it requires concurrency evidence and must not admit duplicate attempts.
5. Split PR-04 into separately reviewable slices. Current order: (a) identity SQLite adapter with existing `agentstate` opener/migration owner and compatibility facades; (b) enrollment adapter with protected-store side effects left in the application service; (c) inventory/proposal/configuration transactions by complete repository operation; (d) candidate/runtime and health ownership only after the apply-port decision. These slices do not change Core or security policy.

### PR-04a identity slice implementation / verification

- Moved installation identity SQL behavior to `internal/identity/sqlite`; `agentstate.NewIdentityStore` and the concrete `SQLiteStore.EnsureInstallationID` remain compatibility entrypoints. `agentstate` still owns schema migration and opens the shared operational DB through `SQLiteStore.OpenDatabase`.
- Added a repository regression test proving first-initialization-wins across the new adapter and compatibility facade. Existing `agentstate` identity, Core Gateway, and enrollment tests also exercise the in-transaction identity read path.
- RED: `go test ./internal/identity/sqlite -count=1` failed before production code because the new adapter package had no implementation.
- GREEN: `go test ./internal/identity/sqlite ./internal/agentstate -run 'TestRepositoryUsesSharedStateDatabaseAndKeepsFirstInstallationID|TestSQLiteIdentity' -count=1` passed on Windows.
- Full Windows suite: `go test ./... -count=1` PASS. Additional gates: `go test -race ./internal/identity/sqlite ./internal/agentstate -count=1`, `go vet ./...`, `go build ./...`, `GOOS=linux GOARCH=amd64 go build ./...`, and `scripts/check-architecture.ps1` PASS. The architecture script must be invoked directly from the active PowerShell shell; launching it through nested `powershell -File` produced a Go template argument parsing error before the script check ran.
- The sandboxed SQLite test attempt failed with SQLite error 14 while creating temporary databases; the same focused tests and full suite passed with the user's normal Windows filesystem access. Linux cross-build is not Linux E2E. No native service/E2E, Docker, Collector, schema migration, or enrollment adapter migration was part of this slice.
- PR #23 targeted `chore/v0.1.4a-next` and merged at `4bdfff72ea1bcc6b344ced4fdc65f5d0b3df3301`. Final-head CI run `43142122120` passed on Ubuntu and Windows. Independent code review found no Critical, Important, or Minor findings. The reusable development branch was recreated and pushed from the merged integration head.

### PR-04b enrollment slice implementation / verification

- Enrollment persistence is being moved to `internal/enrollment/sqlite` behind the existing `enrollment.AttemptRepository`; `agentstate.NewEnrollmentStore` and concrete `SQLiteStore` methods remain compatibility facades. The adapter receives `SQLiteStore.OpenDatabase` and the existing shared `sqliteUpdateMutex`.
- The adapter owns only SQLite attempt metadata and opaque credential references. `enrollment.Service` still owns retry/resume behavior, Core response validation, and the ordering between durable status updates and calls to `CredentialStore.Delete`; `credentialstore` remains unchanged.
- Atomic operations retained: installation-ID check and initial attempt creation; failed-attempt replacement; explicit re-enrollment preserving old binding metadata; terminal completion plus credential reference; permanent failure; clearing the prior credential reference only after external deletion succeeds.
- RED: `go test ./internal/enrollment/sqlite -count=1` failed because the new adapter package had no implementation. GREEN: the focused new adapter test plus existing enrollment persistence tests passed.
- Full Windows suite `go test ./... -count=1`, focused race, `go vet ./...`, Windows build, Linux/amd64 cross-build, and direct `scripts/check-architecture.ps1` invocation passed. The existing Windows SQLite+CredentialStore integration test was explicitly run and SKIPPED because this process is not elevated; no credential-store integration pass is claimed for this slice.
- Independent code review completed with no Critical, Important, or Minor findings. PR #24 targeted `chore/v0.1.4a-next` and merged at `bdc656acc8f6c6cfa3fb09f8cf4cc3d3a8e8e9cc`; CI run `43144356317` passed on Ubuntu and Windows. The reusable branch was recreated from the integration head. No schema migration, Docker/Collector test, or native Linux E2E was part of this persistence-only slice.

### Open decisions (not resolved by this audit)

- Exact owner/API for shared operational DB opening and migration orchestration when context repositories are separated.
- Whether migration files/packages can move without changing migration version/order or opening a new database more than once per operation.
- Whether `reconciliation` should consume the runtime-owned apply port directly or use a consumer-owned port plus composition adapter.
- Target context ownership for `IntegrationInstance`: current type is defined in `internal/integration`, while desired-state and Template use cases are in `internal/configuration`; do not duplicate it.
- Core configuration authority, OpAMP identity/status mapping, and remote-operation security remain outside this checkpoint until their dependent contracts are approved.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Confirm exact domain ownership from source and contracts; record remaining ownership decisions without inventing them. **Audit complete at the base recorded above; open decisions remain.**
2. **Identity adapter slice complete and merged:** use the existing shared SQLite open/migration provider contract and extract identity persistence through compatibility facades; old schema, shared transaction reads, and repeated initialization passed Windows tests and final-head CI. Enrollment remains a separate slice gated on its recovery and protected-store contract.
3. Extract inventory and monitoring proposal repositories as complete operations; preserve complete-successful-snapshot and stale/missing semantics.
4. Extract configuration desired-state/Template/approval adapters; keep approval and desired-state mutation in ONE atomic transaction.
5. Decide the canonical candidate-apply port/DTO owner before altering imports; move generation journal/recovery storage only with the runtime owner and its transaction contract.
6. Extract health projections and platform dependency adapters without changing installation ownership, checksums, retry or teardown semantics.
7. Add package-level `doc.go` and enforce allowed imports after each coherent ownership change. Avoid identical folder nesting unless complexity warrants it.

## Edge cases and failure semantics

- A partial provider snapshot cannot delete active inventory.
- Two writers cannot bypass CAS or create duplicate approvals.
- Old enrollment lost-response/revocation behavior remains identical.
- No duplicate desired IntegrationInstance types during migration.
- Failed candidate preserves LKG and activation journal atomicity.
- Uninstall cannot remove dependencies not installed by Agent.

## Acceptance / verification gates

- All existing contract/SQL fixture/CLI tests pass on old and newly created DBs.
- Cross-package imports obey boundaries; `cmd` not allowed to access raw SQL.
- Windows/Linux builds and Redis/Template OTLP regression re-run after each workflow move.
- Structural changes do not alter acceptance/review rules for service discovery.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Prefer coherent revertable workflow commits; keep old SQL constructors/facades until equivalence confirmed. If PR becomes too large split into PR-04a identity+inventory, 04b monitoring+configuration, 04c runtime+health with same gate; do not merge a big-bang diff.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
