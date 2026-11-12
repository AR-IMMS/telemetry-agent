# PR-04c — Discovery Inventory and Proposal SQLite Adapters

**Status:** PASS — merged into `chore/v0.1.4a-next`  
**Release target:** `v0.1.4a-next`  
**Branch:** `feat/v0.1.4a-next-work`  
**PR:** [#25 — Extract Discovery SQLite adapters](https://github.com/AR-IMMS/telemetry-agent/pull/25)  
**PR base:** `chore/v0.1.4a-next`  
**PR head:** `06a91377c2800f6e0379f779c8e1fa94441d2588`  
**Merge commit:** `7bec19a5dae675778e1670037a8770b8285d5a86`  
**Final-head CI:** run `43146754385` — Ubuntu PASS (46s), Windows PASS (1m49s)  
**Verification date:** 2026-11-12

## Scope and result

Moved the Discovery inventory and monitoring-proposal SQLite adapters from `internal/agentstate` to `internal/discovery/sqlite`. The existing `agentstate.SQLiteStore.InventoryRepository` and `ProposalRepository` methods remain compatibility facades and inject the same shared database opener and in-process write mutex.

The change preserves migration ownership and ordering (schema versions 10–12), complete-provider-snapshot replacement semantics, independent inventory material/write revisions, failure evidence retention, proposal validation and CAS transitions, and the existing cross-table approval transaction in `agentstate`. No schema, CLI, matching, runtime, security-policy, or platform behavior was changed.

## Contract and review evidence

- The inventory/proposal contracts were added to the PR-04 plan before implementation.
- New regressions cover inventory failure evidence and stale write-CAS rejection, and proposal CAS dismissal and repository reopen behavior. Existing agentstate migration/concurrency/approval and runtime tests remain in place.
- Independent standards review found no hard policy violations or actionable code-smell findings.
- Independent spec review found no missing contract, scope creep, or incorrect behavior.
- Code graph impact review identified six affected flows and no test gaps.

## Verification

| Check | Result | Evidence |
|---|---|---|
| `go test ./internal/discovery/sqlite ./internal/agentstate -count=1` | PASS | User's Windows environment |
| `go test ./... -count=1` | PASS | User's Windows environment; all packages passed |
| `go test -race ./internal/discovery/sqlite ./internal/agentstate ./internal/discovery ./internal/configuration -count=1` | PASS | User's Windows environment |
| `go vet ./...` | PASS | User's Windows environment |
| `go build ./...` | PASS | Windows build |
| `GOOS=linux GOARCH=amd64 go build ./...` | PASS | Cross-build from Windows; not native Linux execution |
| `scripts/check-architecture.ps1` | PASS | Package docs, import boundaries, and 450-line guard passed for 29 packages |
| `gofmt -l` on changed Go files | PASS | No unformatted files |
| `git diff --cached --check` | PASS | No whitespace errors before commit |
| Markdown relative links | PASS | All relative links in the updated PR-04 plan, release progress, and PR-04c report resolve |
| Final-head GitHub CI | PASS | Ubuntu and Windows jobs on head `06a91377c2800f6e0379f779c8e1fa94441d2588` |

## E2E and limitations

Docker/Collector E2E and native Linux E2E were not run. This checkpoint only relocates persistence adapters; it does not change runtime collection, services, platform code, Collector configuration, or Docker integration. The Linux cross-build is not native Linux E2E.

The shared engineering policy submodule was not modified. It is absent from this worktree's materialized `docs/shared/` path; the release baseline records that the pinned policy was reviewed previously. This change adds no policy provision.

## Regressions and rollback

No regression was found in the local suite or final-head CI. SQLite schema and migration versions did not change. To revert this checkpoint, revert merge commit `7bec19a5dae675778e1670037a8770b8285d5a86` with its first parent retained.

## Documentation and next checkpoint

Updated the PR-04 migration plan and release progress. The next PR-04 slice is Configuration desired-state/Template/approval adapter ownership. Keep approval and desired-state mutation in one transaction, and define any required adapter/transaction contracts before that slice changes code.
