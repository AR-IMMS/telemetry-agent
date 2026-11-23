# PR-02 — shared SQLite foundation and advisory events

**Canonical release:** `v0.1.4a-next`<br>
**Status:** PASS — local gates and both final-head CI jobs passed; merged<br>
**Base:** PR-01 merge `7ba6efb86218969f539c17bea7a0a7ea6a972169` on `chore/v0.1.4a-next`<br>
**Work branch:** `feat/v0.1.4a-next-work`<br>
**PR / code commit:** [PR #21](https://github.com/AR-IMMS/telemetry-agent/pull/21) / `defa16078e7cb79e6ddff3294328b22aebe32706` (`feat(pr-02): add bounded events and sqlite foundation`)<br>
**Final CI-tested head:** `21958ac9d0ca9a39dcf2029f1d46d2e6bee5eecb`<br>
**Final CI run:** [43132693523](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43132693523) — Ubuntu and Windows passed<br>
**Merge commit:** `d71f5aa39777824e29f1d06a364bb0a289373803`<br>
**Report date:** 2026-11-12

## Goal and scope

PR-02 extracts the shared SQLite connection setup without changing the database schema or domain SQL, and adds a bounded process-local wakeup for a committed desired-state revision. The event is advisory: handlers re-read SQLite, the Agent still reconciles at startup and every 15 seconds, and the standalone CLI remains cross-process with polling as its change detector.

## Contract and source changes

- Added `internal/platform/sqlite`: one-connection pool, existing `WAL`, `synchronous=FULL`, immediate transaction, 5-second busy timeout, parent directory creation, ping, serialized initialization, caller-owned schema/import callback, and cleanup on failure. `agentstate` still owns migration ordering, tables, SQL, legacy JSON import, and mutation locks.
- Added `internal/platform/events`: typed `DesiredConfigurationChanged{Revision}`, one bounded FIFO, nonblocking publish, one serial handler, cancellation, panic/error isolation, and deadline-bounded shutdown.
- Template materialization publishes only after a changed desired revision commits and after releasing `sqliteUpdateMutex`. A queue failure logs a safe code and does not change the successful durable write result. No-op, failed CAS, and rollback paths emit no event.
- The Agent Service composes that dispatcher with the existing reconciliation controller. Poll/startup reconciliation remains intact for cross-process writes, process restart, crash after commit, and queue overflow.
- No schema, migration ledger, CLI command/output contract, dependency, Collector, enrollment, credential, or shared-submodule change was made.

## Verification

| Check | Environment | Result |
|---|---|---|
| `go test ./... -count=1` | User's Windows environment | PASS; all Go packages passed |
| `go test -race ./internal/platform/... ./internal/agentstate` | User's Windows environment | PASS |
| `go test -race ./internal/platform/events` after cancellation coverage | User's Windows environment | PASS |
| `go vet ./...` | Windows worktree | PASS |
| `go build ./...` | Windows worktree | PASS |
| `GOOS=linux GOARCH=amd64 go build ./...` | Windows host, Linux target | PASS; cross-build only |
| `pwsh -NoProfile -File scripts/check-architecture.ps1` | Windows source view | PASS; 25 packages, docs present, boundaries pass, 271 production files within limit |
| Same architecture script with `GOOS=linux` | Windows host, Linux source view | PASS |
| `git diff --check` | Worktree | PASS |
| Markdown link validation | Four changed release documents | PASS; 8 relative links, 0 missing targets |
| Actual pinned Collector archive checksum, version, and component probe | PR-02 GitHub Actions, Ubuntu and Windows | PASS on final head `21958ac9d0ca9a39dcf2029f1d46d2e6bee5eecb`, run `43132693523` |

The new SQLite and dispatcher API tests were run first against the absent package APIs and failed to compile as expected; implementation then made them pass. This verifies test-first coverage was established before the new implementation.

The first sandboxed SQLite run returned `unable to open database file (14)` because the sandbox denied temp database creation. The same focused and full test commands were rerun in the user's Windows environment and passed.

The migration ledger remains version 13 in existing tests. Existing fixtures and tests for old-state loading, migration order, WAL, CAS conflict, idempotence, and legacy import passed as part of the full suite.

## Infrastructure and platform evidence

- Docker Engine 29.5.3 and Compose 5.1.4 were available from the earlier user-environment check. No Docker infrastructure scenario is required for this persistence/event-only checkpoint; no containers or existing stacks were changed.
- Native Linux execution, Windows/Linux service E2E, and gateway telemetry delivery were NOT RUN. Linux evidence here is a cross-build plus CI source/manifest checks, not native Linux E2E.
- The local environment did not provide an extracted Collector binary for a fresh component probe. The PR CI manifest probe is a required merge gate.

## Regressions, security, and limitations

- Event payloads contain only a positive revision number. Handler and queue failures are logged using safe codes without logging the event error or domain data.
- Event delivery is in-memory, at-most-once per admitted queue item, and may be dropped on shutdown or queue saturation. Startup and periodic SQLite reconciliation repair missed wakeups.
- `agentstate` remains a broad SQL owner; this checkpoint deliberately does not split tables or repositories across contexts.
- The existing Git worktree artifact `cmd/agentctl/otel.yaml.lifecycle.lock` is untracked and unexplained. It was preserved and excluded from this change.

## Changed files

- `internal/platform/sqlite/{doc.go,sqlite.go,sqlite_test.go}`
- `internal/platform/events/{doc.go,dispatcher.go,dispatcher_test.go}`
- `internal/agentstate/sqlite_store.go`
- `internal/agentstate/sqlite_materialization.go` and `sqlite_materialization_test.go`
- `cmd/agentctl/reconciliation_runtime.go`
- `cmd/agentctl/template_materialization_runtime.go`
- `docs/local/plans/v0.1.4a-next/pr-02-events-sqlite.md`
- `docs/local/plans/v0.1.4a-next/release-progress.md`
- `docs/local/verification/v0.1.4a-next/pr-verification/pr-01-report.md`
- This report

## Rollback and next checkpoint

Disable the optional materialization publisher and keep the dispatcher out of runtime composition; existing SQLite polling remains. The connection factory can be reverted to `SQLiteStore.open` without data downgrade because schema and persisted rows are unchanged.

PR #21 was marked ready and merged after both Windows and Ubuntu passed on the final report-sync head, including exact pinned Collector artifact/component checks. PR-03 contract work may now begin; establish CLI/use-case boundaries before dependent implementation.
