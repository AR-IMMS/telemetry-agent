# PR-02 — Shared SQLite Foundation & Typed Event Dispatcher

**Status:** VERIFYING — implementation and local verification complete; integration CI and merge remain. **Release target:** `v0.1.4a-next`. **Evidence:** current feature branch starts at PR-01 merge `7ba6efb86218969f539c17bea7a0a7ea6a972169`.

## Objective and exit boundary

Extract technical SQLite primitives without changing schema, then add an optional typed in-process event dispatcher as *post-commit wakeup only*.

## Baseline / current architecture constraints

`agentstate` currently owns cross-domain SQL and shared DB opening; inventory/materialization/approval/reconciliation rely on SQLite CAS and periodic recovery. Preserve existing 15-second reconciliation fallback until a separately approved behavior change.

## PR-02 contract decisions verified before implementation

These contracts narrow this checkpoint without deciding later context ownership:

- `internal/platform/sqlite` provides the shared SQLite connection factory: create the parent directory, open the existing mattn SQLite driver with the existing WAL, `synchronous=FULL`, 5-second busy timeout and immediate transaction defaults, cap the pool at one connection, ping, initialize through a caller-supplied schema/import callback, and close the handle on initialization failure. It does not own tables, SQL migrations, legacy JSON import semantics, or domain transaction locks. `agentstate` remains the compatibility facade and migration/table owner.
- Keep `SQLiteStore` value construction and all existing repository interfaces intact. No schema version, table, row encoding, migration ordering, CAS rule, or state-file import behavior changes. The current migration ledger is version 13.
- `internal/platform/events` is process-local and advisory. PR-02 registers a fixed `DesiredConfigurationChanged` event carrying only the committed desired revision (`uint64`); no payloads, secrets, config graphs, or commands. Handler re-reads authoritative SQLite state. The dispatcher has one serial bounded queue/handler, nonblocking publish (`ErrQueueFull`), one start, cancellation, and context-bounded shutdown. Handler errors/panics are logged as safe codes and isolated. Publish is never performed while holding the SQLite transaction or `sqliteUpdateMutex`.
- The only producer in this checkpoint is `agentstate.materializationSQLite.ApplyTemplateProjection`: publish once, after commit and after releasing `sqliteUpdateMutex`, only when the desired revision changed. Queue failure cannot turn a committed repository write into a reported write failure. No-op/CAS-failed/rolled-back operations emit nothing.
- The Agent Service runtime owns discovery/materialization and reconciliation in the same process (`cmd/agentctl/reconciliation_runtime.go`, `discovery_runtime.go`); its reconciliation controller will accept an optional wakeup signal and still reconcile at startup and every 15 seconds. The standalone `agentctl` commands are separate processes and do not use this in-memory channel.
- Missed events, service restart, queue overflow, and process crash after commit recover through startup and periodic SQLite polling. Duplicate/out-of-order revisions are hints only; reconciliation reads the latest durable snapshot and existing CAS/idempotency behavior remains authoritative.

### Verified current source mapping

- SQLite open and migration callback: `internal/agentstate/sqlite_store.go`, `sqlite_schema.go`; only this production file calls `sql.Open`.
- Domain commits/CAS: `internal/agentstate/sqlite_materialization.go`, `sqlite_approval.go`, `sqlite_integration.go`, `sqlite_reconciliation.go`; shared process mutexes remain in `sqlite_store.go` for now.
- Runtime producer and consumer composition: `cmd/agentctl/template_materialization_runtime.go`, `discovery_runtime.go`, `reconciliation_runtime.go`; `internal/reconciliation/controller.go` currently performs a startup pass plus a 15-second ticker.
- Existing tests already exercise template projection, CAS conflict, idempotence, and migration fixtures under `internal/agentstate`; these remain compatibility gates.

### Implemented source mapping

- `internal/platform/sqlite/{doc.go,sqlite.go}` owns connection setup defaults and initialization serialization; `agentstate.SQLiteStore.open` remains the domain facade and calls the existing schema/import logic unchanged.
- `internal/platform/events/{doc.go,dispatcher.go}` provides the fixed typed event, bounded FIFO admission, one handler, cancellation, and deadline-bounded shutdown.
- `internal/agentstate/sqlite_materialization.go` emits after a changed projection commits and releases `sqliteUpdateMutex`; `cmd/agentctl/reconciliation_runtime.go` composes the dispatcher with the service reconciler; `template_materialization_runtime.go` supplies it only to the service-owned materializer.
- New tests cover SQLite defaults/initializer cleanup, queue ordering and overflow, handler error/panic isolation, cancellation/shutdown, concurrent publish, commit-before-publish, no-op/CAS suppression, and durable success when enqueue fails.

### Explicitly deferred

This event contract does not define event contracts for inventory snapshots, policy changes, Collector lifecycle, health, OpAMP, Core, or remote operations. It does not establish a durable outbox, cross-process notification, generic plugin/event framework, or a command queue.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: SQLite platform + configuration/reconciliation + app host.

## Implementation steps (ordered)

1. Baseline SQLite open/close, migration ordering, WAL/busy-timeout, transaction and lock semantics using old DB fixtures; write failing compatibility tests first.
2. Create `internal/platform/sqlite` with the connection factory described above; keep `agentstate` compatibility facade and leave migration/table-owning SQL and transaction locks in place. Do not rename tables or split the database.
3. Create `internal/platform/events` typed bounded dispatcher with fixed registration, bounded admission, error handling, cancellation and deadline-bounded shutdown; no secrets or data graphs in events.
4. Publish only after successful durable state commits for one controlled path (e.g., desired-state revision). Handler re-reads SQLite; wire advisory wakeup while keeping startup and poll recovery.
5. Add observability for dropped/queue-full events and handler failure without failing already-committed writes.

## Edge cases and failure semantics

- Power loss after DB commit before publish must recover by poll.
- Duplicate/late events must not regress newer revision.
- Handler panic/error must not kill Agent.
- Queue full, recursion, shutdown while busy, cancellation and concurrent publish must be bounded.
- Concurrent DB mutations retain CAS; no hidden nested transaction lock.

## Acceptance / verification gates

- Old DB fixtures open without schema/data changes, including credentials/legacy JSON where applicable.
- Transaction boundary, lock and concurrency tests pass; poll-only mode still converges.
- Dispatcher tests cover ordering, queue-full, cancellation, panic/error isolation, idempotence, lost-event recovery.
- Windows/Linux build and existing Redis/Collector tests show no change.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths. See [`pr-02-report.md`](../../verification/v0.1.4a-next/pr-verification/pr-02-report.md).

## Rollback and stop conditions

Feature-flag event subscriptions off and keep polling; revert adapter extraction without DB downgrade. Record that lost events are expected and recovered by polling.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
