# PR-03 — Application Host & Thin CLI

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Move composition and runtime control loops from `cmd/agentctl` to `internal/app` without changing command contracts or runtime ordering.

## Baseline / current architecture constraints

Audit identified CLI composition across `composition.go`, `discovery_runtime.go`, `reconciliation_runtime.go`, Template/approval wiring. App is composition root only; domain decisions stay in owning contexts.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Inventory command families, CLI help/JSON/text exit codes and effective run/service lifecycle; record golden snapshots.
2. Create `internal/app` entrypoints for host startup/shutdown and dependency construction. Pull one workflow at a time, retaining `cmd/agentctl` wrappers and stable flags.
3. Inject context-owned ports for enrollment, discovery, monitoring materialization, reconciliation and runtime instead of passing the global SQLite store everywhere.
4. Tie event dispatcher and loops to one host `context.Context`; preserve old schedules/retries and lifecycle lock ordering.
5. Document command composition, cancellation and resource cleanup; add subprocess tests for service stop and concurrent CLI state writes.

## Edge cases and failure semantics

- Foreground CLI invocation must not initialize long-running poll loops accidentally.
- Windows SCM and Linux systemd commands gracefully stop; no leaked Collector children.
- Command-as-separate-process changes remain detectable by SQLite polling.
- Startup failure mid-wiring cleans up started resources.

## Acceptance / verification gates

- CLI snapshots and exit codes match baseline.
- Service install/start/stop/reboot orchestration tests pass on native OS where available.
- Docker discovery → Template → Redis activation regression remains equivalent.
- No domain logic or SQL records added to `cmd/agentctl` or `internal/app`.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Leave CLI command facade and re-route its host constructor to old wiring if needed; no persisted-state change.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
