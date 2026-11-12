# PR-03 — Application Host & Thin CLI

**Status:** PASS — PR #22 merged after local verification, live Windows Docker/Collector E2E, and final-head Windows/Ubuntu CI. **Release target:** `v0.1.4a-next`. **Merge:** `eaaedc8ef229113573903bce9fa72c608025628d`. The reusable feature branch has been recreated from the updated integration head.

## Objective and exit boundary

Move composition and runtime control loops from `cmd/agentctl` to `internal/app` without changing command contracts or runtime ordering.

## Baseline / current architecture constraints

Audit identified CLI composition across `composition.go`, `discovery_runtime.go`, `reconciliation_runtime.go`, Template/approval wiring. App is composition root only; domain decisions stay in owning contexts.

## PR-03 contracts verified before implementation

- `cmd/agentctl` remains the executable and presentation adapter: root dispatch, argument/flag parsing, text/JSON formatting, exit codes, signal mapping, service commands, and the current `dependencies` test seam remain stable. Preserve the current `agentctl help` snapshot, including the existing `service` omission; do not repair it incidentally.
- `internal/app` owns only Agent Service composition and long-running control-loop lifecycle. It may construct repository adapters and connect domain application services, but it must not own domain rules, CLI output, configuration or persistence SQL, or OS-specific service-manager behavior.
- The only PR-03 host workflow to move is the existing `runRuntimeWithReconciliation` composition: baseline/reconciliation controller, Docker discovery coordinator, proposal refresh, Template materialization, event dispatcher, and supervised control-loop shutdown. Move runtime-only collaborators from `cmd/agentctl` as needed; keep explicit CLI proposal approval, enrollment, install/uninstall, and dependency command handlers in `cmd/agentctl`.
- The composition seam is one context-aware `app.RunRuntime(ctx, options)` call. Options carry the existing state path, teardown callback, health reporter, and command runner. The application host owns one child context for dispatcher, discovery, and reconciliation; cancellation stops them in a bounded order. Preserve the 15-second reconciliation fallback and 30-second Docker discovery schedule.
- CLI subcommands other than the long-running Agent Service must not start discovery, Template refresh, or reconciliation loops. The existing Agent Service invokes `app.RunRuntime`; CLI commands continue to invoke their current finite use cases.
- Repository adapters remain domain ports; PR-03 does not split SQLite tables, change schema, or relocate SQL. Do not introduce generic service locators, plugin systems, or command buses.

### Verified current source mapping

- CLI entrypoint and dispatch: `cmd/agentctl/main.go`; composition test seam/default wiring: `cmd/agentctl/composition.go`.
- Long-running host composition: `cmd/agentctl/reconciliation_runtime.go`, `discovery_runtime.go`, and runtime portions of `proposal_approval_runtime.go` / `template_materialization_runtime.go`.
- Service and CLI commands remain in `cmd/agentctl/lifecycle_commands.go`, `service_windows.go`, `service_other.go`, and command-family files. Existing tests include help/exit contracts and runtime lifecycle/discovery/materialization regressions.
- PR-02 event dispatcher is composed only by the long-running service host; SQLite polling remains the fallback for separate CLI processes.

### PR-03 implementation mapping

- Added `internal/app` as the Agent Service host. It owns bounded dispatcher lifecycle, proposal/template/discovery/reconciliation loop coordination, and the existing 15-second reconciliation / 30-second discovery intervals.
- `cmd/agentctl/reconciliation_runtime.go` now adapts the existing controller, Docker coordinator, proposal refresher, and Template materializer into `app.RunRuntime`. Existing repository adapters and CLI use cases remain in their current owners.
- `cmd/agentctl/discovery_runtime.go` retains Docker provider construction; long-running loop coordination moved to `internal/app`. Existing discovery-cycle assertions now use the app-owned predicate.
- `internal/platform/events.Publisher` makes the event publishing port explicit; the materializer constructor accepts that contract rather than an anonymous interface.
- Moved runtime-loop tests from `cmd/agentctl` to `internal/app`. The finite CLI test suite and the prior gated live Template runtime test remain in the CLI package.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Inventory command families, CLI help/JSON/text exit codes and effective run/service lifecycle; record golden snapshots.
2. Create `internal/app` entrypoints for host startup/shutdown and dependency construction. Pull one workflow at a time, retaining `cmd/agentctl` wrappers and stable flags.
3. Keep existing context-owned repository interfaces at composition seams; do not perform the broader context/repository migration scheduled for PR-04.
4. Tie event dispatcher and loops to one host `context.Context`; preserve schedules, retries, and lifecycle lock ordering.
5. Document command composition, cancellation, and resource cleanup; add regression tests for finite CLI commands, service stop, and concurrent CLI state writes.

## Edge cases and failure semantics

- Foreground CLI invocation must not initialize long-running poll loops accidentally.
- Windows SCM and Linux systemd commands gracefully stop; no leaked Collector children.
- Command-as-separate-process changes remain detectable by SQLite polling.
- Startup failure mid-wiring cleans up started resources.

## Acceptance / verification gates

- CLI tests and the preserved help/output contracts pass locally and in final-head CI.
- Local unit/integration coverage for app loop coordination passes. Native service install/start/stop/reboot E2E is not claimed by this refactor.
- Docker discovery → Template → Redis activation passed in the live Windows runtime regression using the pinned Collector and isolated Redis fixtures.
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
