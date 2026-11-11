# PR-04 — Bounded Context Ownership & Repository Migration

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Align package ownership with seven contexts (six established plus deferred Operations) while preserving SQLite schema, public CLI and all runtime behavior. This is a source-refactoring PR, not a new feature.

## Baseline / current architecture constraints

Audit found a 35-file `agentstate`, split IntegrationInstance ownership and `reconciliation -> agentlifecycle` inversion. Its ownership matrix is a review input, not proof every proposed move is already safe.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Confirm and document exact domain ownership: `identity` install/enrollment; `inventory` resource identity; `monitoring` definitions/matching; `configuration` desired instances/Templates/CAS; `runtime` process/generation/LKG; `health` projections.
2. Extract identity/enrollment SQLite adapters first through compatibility facades; verify old DB + secret stores.
3. Extract inventory and monitoring proposal repos, preserve complete-successful-snapshot and stale/missing semantics.
4. Extract configuration desired-state/Template/approval adapters; keep approval and desired-state mutation in ONE atomic transaction.
5. Invert `reconciliation -> agentlifecycle` using a minimal candidate-apply port; move generation journal/recovery storage with runtime owner.
6. Extract health projections and platform dependency adapters without changing installation ownership, checksums, retry or teardown semantics.
7. Add package-level `doc.go` and enforce allowed imports. Avoid identical folder nesting unless complexity warrants it.

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
