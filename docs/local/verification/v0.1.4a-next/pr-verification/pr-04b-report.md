# PR-04b — Enrollment SQLite adapter

**Checkpoint status:** Local implementation and verification PASS; independent review PASS; GitHub CI and merge pending.

**Release target:** `v0.1.4a-next`

**Branch:** `feat/v0.1.4a-next-work`

**Base SHA:** `376b8526b0132d3dca1f9eabb9c9ffe9a9f01f5d` (PR-04a post-merge documentation sync)

**Implementation commit:** pending

## Scope and contract

Moved enrollment attempt persistence into `internal/enrollment/sqlite` behind the existing `enrollment.AttemptRepository`. `agentstate.NewEnrollmentStore` and concrete `SQLiteStore` persistence methods remain compatibility facades. `agentstate` still owns the shared database opener, schema migration order, and shared in-process write mutex; SQLite transactions and the configured busy timeout continue to coordinate other processes.

The adapter stores only secret-free request/result metadata and opaque credential references. Enrollment application logic continues to own retry/resume, Core response validation, protected credential operations, and cleanup ordering. No Core/security policy, schema, or credential-store behavior changed.

## Changed files

- `internal/enrollment/sqlite/doc.go`
- `internal/enrollment/sqlite/repository.go`
- `internal/enrollment/sqlite/repository_test.go`
- `internal/agentstate/repository.go`
- `internal/agentstate/sqlite_enrollment.go`
- `docs/local/plans/v0.1.4a-next/pr-04-context-migration.md`
- `docs/local/plans/v0.1.4a-next/release-progress.md`
- This verification report

The unrelated zero-byte `cmd/agentctl/otel.yaml.lifecycle.lock` remains untracked and was not edited or staged. No file under `docs/shared/` changed.

## Verification

| Check | Result | Evidence / limitation |
|---|---|---|
| New adapter regression, RED | PASS | Before implementation, `go test ./internal/enrollment/sqlite -count=1` failed because the new adapter package had no implementation. |
| Focused adapter and repository recovery tests | PASS | `go test ./internal/enrollment/sqlite ./internal/agentstate -run 'TestRepositorySharesOperationalDatabaseAndCompatibilityFacade|TestSQLiteEnrollment' -count=1` |
| Full Windows Go suite | PASS | `go test ./... -count=1` |
| Race tests | PASS | `go test -race ./internal/enrollment/sqlite ./internal/agentstate ./internal/enrollment -count=1` |
| Static analysis | PASS | `go vet ./...` |
| Windows build | PASS | `go build ./...` |
| Linux/amd64 cross-build | PASS | `GOOS=linux GOARCH=amd64 go build ./...`; this does not establish native Linux behavior. |
| Architecture guard | PASS | Direct invocation `& ./scripts/check-architecture.ps1`: 28 source directories/packages and 277 production files within configured limits. |
| Windows SQLite + CredentialStore integration | SKIPPED | Explicit run `go test ./internal/enrollment -run '^TestSQLiteAndCredentialStoreKeepEnrollmentSecretsSeparated$' -count=1 -v`; fixture reported that this process is not elevated. No credential-store integration PASS is claimed. |
| Independent code review | PASS | No Critical, Important, or Minor findings. |
| GitHub final-head CI | NOT RUN | Checkpoint is not yet submitted. |
| Native Linux E2E | NOT RUN | Linux cross-build only. |

## Risks and remaining decisions

- `agentstate` remains the shared schema/migration owner. Its global mutex is injected into the enrollment adapter to preserve process-wide mutation serialization.
- The explicit Windows credential-store integration test did not run because the current process lacks the required elevation. Existing pure and SQLite persistence suites passed.
- Native Linux E2E remains a release-level gate.

## Rollback and next step

Revert the enrollment adapter/facade extraction as one unit. No persisted schema or data migration occurred, so rollback requires no database restoration. Continue to final review, PR CI, and merge only after those gates pass.
