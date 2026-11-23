# PR-04d — Configuration SQLite Repositories

| Field | Result |
|---|---|
| Status | PASS — merged into `chore/v0.1.4a-next` |
| Release target | `v0.1.4a-next` |
| Branch | `feat/v0.1.4a-next-work` |
| PR | [#26 — Move Configuration SQLite adapters](https://github.com/AR-IMMS/telemetry-agent/pull/26) |
| PR base | `chore/v0.1.4a-next` |
| PR head | `5bb7200c5b10cd9b15f2451d346d652acbafd1e9` |
| Merge commit | `b583955249e9d6acbc3c814fe320216574ed6faf` |
| Final-head CI | Run `43149784899` — Ubuntu PASS (47s), Windows PASS (1m31s) |
| Verification date | 2026-11-12 |

## Scope and result

Moved desired Integration, Template, materialization, and proposal-approval SQLite adapters from `internal/agentstate` to `internal/configuration/sqlite`. `agentstate.SQLiteStore` retains compatibility constructors and supplies the shared database opener and in-process write mutex.

The existing table definitions and migration versions 6 and 13 remain under `agentstate`. Desired/Template item and set revisions, no-op behavior, tombstones, materialization CAS, approval idempotency, cross-table transaction boundaries, and post-commit advisory event behavior are preserved. `IntegrationInstance` remains owned by `internal/integration`; the candidate DTO and candidate-state persistence ownership were not changed.

This PR also carries the PR-04c post-merge roadmap/report closeout commits that were committed to the reusable branch after PR #25 merged.

## Contract and review evidence

- PR-04d repository and transaction contracts were recorded in the PR-04 plan before implementation.
- Direct adapter tests cover shared database access, desired/Template CAS and no-op behavior, approval persisting both the proposal and desired instance, and materialization publishing only after the desired snapshot is committed.
- Existing facade tests continue to cover approval rollback/uniqueness, materialization rollback/revision conflicts, migration/reopen compatibility, and concurrent repository behavior.
- Independent standards review initially requested constructor GoDoc for required opener/lock dependencies; the comments were updated and follow-up review confirmed the fix. No other standards findings remain.
- Independent spec review found no contract or behavior findings.

## Verification

| Check | Result | Evidence |
|---|---|---|
| `go test ./internal/configuration/sqlite ./internal/agentstate ./internal/configuration -count=1` | PASS | User's Windows environment |
| `go test ./... -count=1` | PASS | User's Windows environment; all packages passed on final test set |
| `go test -race ./internal/configuration/sqlite ./internal/agentstate ./internal/configuration ./internal/reconciliation -count=1` | PASS | User's Windows environment |
| `go vet ./...` | PASS | After final direct adapter tests |
| `go build ./...` | PASS | Windows build |
| `GOOS=linux GOARCH=amd64 go build ./...` | PASS | Cross-build from Windows; not native Linux execution |
| `scripts/check-architecture.ps1` | PASS | Package docs, import boundaries, and 450-line guard passed for 30 packages |
| `gofmt -l` on changed Go files | PASS | No unformatted files |
| `git diff --check` | PASS | No whitespace errors in the PR-04d changes |
| Markdown relative links | PASS | PR-04 plan, release-progress, and verification report links resolve |
| Final-head GitHub CI | PASS | Ubuntu and Windows jobs on head `5bb7200c5b10cd9b15f2451d346d652acbafd1e9` |

## Change-analysis note

The code graph reports possible test-association gaps for methods reached through repository interfaces and `agentstate` compatibility facades (risk score 0.86, 17 possible gaps). Manual source/test mapping found the existing facade rollback, migration, approval, materialization, event, and concurrency tests, and the new direct adapter tests exercise the new package constructors. All mapped tests pass locally and on final-head CI; the graph result remains a limitation of automated interface-dispatch associations rather than an untested behavior claim.

## E2E and limitations

Docker/Collector E2E and native Linux E2E were not run. This checkpoint relocates persistence adapters and changes no runtime collection, platform service, Collector configuration, or Docker behavior. The Linux cross-build does not establish Linux runtime compatibility.

The shared engineering policy submodule was not modified. It is absent from this worktree's materialized `docs/shared/` path; the release baseline records that the pinned policy was reviewed previously. This change adds no policy provision.

The `agentstate` candidate-state adapter retains its narrow desired-revision query because the candidate apply/storage ownership decision remains open. No Core authority, OpAMP mapping, remote-operation security, or other unresolved policy was introduced.

## Regressions and rollback

No regression was found in local tests or final-head CI. No SQLite migration or table change was made. To revert this checkpoint, revert merge commit `b583955249e9d6acbc3c814fe320216574ed6faf` with its first parent retained.

## Documentation and next checkpoint

Updated the PR-04 plan and release progress and added this implementation/verification report. The next PR-04 slice is candidate/runtime and health ownership; decide the candidate-apply port and DTO owner before changing imports or moving its recovery journal.
