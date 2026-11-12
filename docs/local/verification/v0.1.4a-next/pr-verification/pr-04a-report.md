# PR-04a — Installation identity SQLite adapter

**Checkpoint status:** Local implementation and verification PASS; GitHub CI and merge NOT RUN.

**Release target:** `v0.1.4a-next`

**Branch:** `feat/v0.1.4a-next-work`

**Base SHA:** `6a5fcf9324bb5acb291034a4dc3ab84d6ce7429e`
**Implementation commit:** pending

## Scope and contract

Moved installation identity persistence behind `internal/identity/sqlite` while retaining `agentstate` as the shared operational database opener and schema migration owner. `agentstate.NewIdentityStore` and `SQLiteStore.EnsureInstallationID` remain compatibility facades. Core Gateway validation reads identity through the adapter using the caller's existing transaction.

Preserved contracts: valid UUIDs only; first initialization wins; repeated/concurrent calls return the committed identity; invalid or missing persisted identity fails; the shared database path, migration order, and schema remain unchanged. Enrollment and other context repositories were not moved.

## Changed files

- `internal/identity/sqlite/doc.go`
- `internal/identity/sqlite/repository.go`
- `internal/identity/sqlite/repository_test.go`
- `internal/agentstate/repository.go`
- `internal/agentstate/sqlite_identity.go`
- `internal/agentstate/sqlite_store.go`
- `internal/agentstate/sqlite_core_gateway.go`
- `docs/local/plans/v0.1.4a-next/pr-04-context-migration.md`
- `docs/local/plans/v0.1.4a-next/release-progress.md`
- This verification report

The unrelated zero-byte `cmd/agentctl/otel.yaml.lifecycle.lock` remains untracked and was not staged or edited. No file under `docs/shared/` changed.

## Verification

| Check | Result | Evidence / limitation |
|---|---|---|
| New adapter regression, RED | PASS | Before implementation, `go test ./internal/identity/sqlite -count=1` failed because the new adapter package had no implementation. |
| Focused identity SQLite regressions | PASS | `go test ./internal/identity/sqlite ./internal/agentstate -run 'TestRepositoryUsesSharedStateDatabaseAndKeepsFirstInstallationID|TestSQLiteIdentity' -count=1` |
| Full Windows Go suite | PASS | `go test ./... -count=1` |
| Race tests | PASS | `go test -race ./internal/identity/sqlite ./internal/agentstate -count=1` |
| Static analysis | PASS | `go vet ./...` |
| Windows build | PASS | `go build ./...` |
| Linux/amd64 cross-build | PASS | `GOOS=linux GOARCH=amd64 go build ./...`; this does not establish native Linux behavior. |
| Architecture guard | PASS | `scripts/check-architecture.ps1` run directly in the active PowerShell shell: 27 source directories/packages, 275 production files within the configured limit. |
| Documentation links and whitespace | PASS | Changed Markdown relative links: 0 missing; `git diff --check` clean. |
| Independent code review | PASS | No Critical, Important, or Minor findings. |
| Windows/Linux native service E2E | NOT RUN | Outside this persistence-only slice. Linux cross-build is not Linux E2E. |
| GitHub final-head CI | NOT RUN | Checkpoint branch has not yet been submitted. |

The sandboxed SQLite test attempt failed with SQLite error 14 while creating temporary databases. The same focused tests and full suite passed in the user's normal Windows environment. Launching the architecture script through a nested PowerShell process also produced a Go template argument parsing error; direct invocation in the active shell passed.

## Risks and remaining decisions

- `agentstate` still owns the shared DB opener and all schema migrations. This slice intentionally preserves that arrangement; a later storage contract may decide whether to extract the provider.
- Enrollment recovery and protected credential store effects are not changed or verified as a migrated adapter.
- The ownership of the reconciliation/runtime candidate-apply port and the `IntegrationInstance` type remains open in the PR-04 plan.
- Native Linux E2E remains a release-level gate.

## Rollback and next step

Revert the identity adapter/facade code changes as a unit. No persisted schema or data migration occurred, so rollback requires no database restoration. Keep the checkpoint open until final-head GitHub CI and branch review are complete; then continue with the separately gated enrollment persistence slice.
