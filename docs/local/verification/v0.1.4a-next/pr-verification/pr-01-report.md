# PR-01 — foundation checkpoint report

**Canonical release:** `v0.1.4a-next`<br>
**Status:** BLOCKED<br>
**Base commit:** `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`<br>
**Branch / PR / Commit SHA:** `feat/v0.1.4a-next-work` → `chore/v0.1.4a-next` / no PR created / implementation commit `75aaf9a36f95778effb70a06750b743d2b3f562e`; base `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`<br>
**Integration branch:** `chore/v0.1.4a-next` (local, still at the base commit)<br>
**Report date:** 2026-11-12

## Summary

PR-01 establishes the reviewed release documentation, preserves current runtime contracts, documents the source baseline, adds a narrow CI architecture/package-documentation/file-size guard, and enables CI for pull requests targeting the approved integration branch. No runtime behavior, SQLite schema, Collector artifact, dependency ownership, enrollment behavior, or shared-submodule content changed. Local checkpoint commit `75aaf9a36f95778effb70a06750b743d2b3f562e` was created after verification. No push or pull request was created.

The user-authorized PR-01 preparation is complete in the isolated worktree. Review and publication remain blocked by invalid GitHub authentication/API reachability and inability to verify the submodule worktree metadata. The shared policy text was readable from the original checkout and is consistent with the local supplement; its checked-out commit/content state could not be certified. Docker Engine access is available with elevated read-only access, but no isolated PR-01 infrastructure scenario was defined or run. Therefore PR-01 is not marked fully PASS for integration.

## Work completed

- Integrated the previously reviewed v0.1.4a-next architecture, CLI contract, requirements, plans, coding-policy supplement, runbook, and verification matrix into the preserved `docs/local/` structure. Updated `docs/README.md` and `docs/ARCHITECTURE.md` to identify v0.1.4a-next as the target architecture.
- Added [`release-progress.md`](../../../plans/v0.1.4a-next/release-progress.md) and this checkpoint report. Updated the plan index and PR-01 plan so the earlier blanket implementation hold is marked superseded by the user’s 2026-11-12 authorization; later PRs remain conditional.
- Added [`internal/agenthealth/doc.go`](../../../../../internal/agenthealth/doc.go) describing the package's local-health responsibility and the limit that readiness does not prove telemetry delivery.
- Added [`scripts/check-architecture.ps1`](../../../../../scripts/check-architecture.ps1): internal packages cannot import the CLI composition root; selected domain/application contexts cannot import persistence/runtime/transport adapters; each Go source directory needs `doc.go`; non-test, non-generated production Go files are capped at 450 lines.
- Recorded two precise transition allowances: `internal/identity` currently imports Windows registry APIs, and `internal/enrollment/service.go` is already 454 lines. The file-specific ceiling is 454 and rejects further growth. These allowances preserve current behavior and do not mandate the proposed source tree.
- Updated [CI](../../../../../.github/workflows/ci.yml) to run on pushes to and pull requests targeting `chore/v0.1.4a-next`. The new guard is in the existing Windows/Linux CI matrix.

## Contract baseline applied

- Actual `agentctl help` output and exit code are captured in [`pr-01-baseline.md`](pr-01-baseline.md). It lists current commands; `service` is routed in source but omitted from root help. `agentctl --help` currently exits 2. PR-01 leaves both behaviors unchanged.
- Existing `--state-path`, revision/CAS options, integration/template commands, output behavior, and old CLI test coverage remain compatibility constraints.
- The SQLite migration ledger is unchanged; source tests assert migration version 13. No migration or table change was made.
- The Collector manifest remains at `otelcol-contrib` 0.160.0 with per-artifact SHA256 validation. No Collector configuration or dependency changes were made.
- Core/local configuration authority, OpAMP identity/status mapping, and controlled-operation authorization remain open for their dependent checkpoints; PR-01 makes no decision on them.

## Tests and verification

| Command / check | Environment | Expected | Actual | Result |
|---|---|---|---|---|
| `go test ./... -count=1` | Windows worktree | All packages pass | 23 packages passed; normal-temp rerun passed after sandbox file/SQLite permission errors | PASS |
| `go vet ./...` | Windows worktree | No diagnostics | Exit 0 | PASS |
| `go build ./...` | Windows worktree | Build succeeds | Exit 0 | PASS |
| `GOOS=linux GOARCH=amd64 go build ./...` | Windows host, Linux/amd64 target | Cross-build succeeds | Exit 0; not native Linux evidence | PASS |
| `pwsh -NoProfile -File scripts/check-architecture.ps1` | Windows Go package view | No forbidden imports, missing package docs, or size violations | 23 packages, 23 `doc.go` directories, 267 production files; documented transition allowances only | PASS |
| Same architecture script with `GOOS=linux` | Windows host, Linux Go package view | Target import graph passes | Same 23 package paths pass | PASS |
| `gofmt -d internal/agenthealth/doc.go` | Windows worktree | No formatting diff | No output | PASS |
| `git diff --check` | Isolated worktree | No whitespace errors | No errors | PASS |
| Markdown link scan | Release docs + entrypoints; full `docs/` audit | New/updated release-doc paths resolve | Scoped: 33 Markdown files / 60 local links, 0 broken; 2 shared-submodule links unavailable. Full audit: 101 files / 137 local links, 4 pre-existing broken historical links and the same 2 unavailable shared links | PARTIAL |
| `go run ./cmd/agentctl help` and `go run ./cmd/agentctl --help` | Windows worktree | Capture existing command output and exit behavior | `help`: exit 0; `--help`: exit 2, unknown command. Output is in `pr-01-baseline.md` | PASS |
| Read shared coding policy | Original checkout; target gitlink inspected | Read and compare policy | Policy text was readable in the original checkout and is consistent with the local supplement; target gitlink is `c4cd59a9bcaaf0735c3107424f1be98fc498670a`, but checked-out submodule worktree status could not be verified | PARTIAL |
| Read-only GitHub checks | `gh` 2.96.0 | Authenticated repo/API access | `GH_TOKEN` exists but is invalid; saved account token invalid; `gh repo view` and `gh api user --jq .login` cannot connect | BLOCKED |
| Docker engine probe (release readiness) | Windows host; elevated read-only access | Docker client and engine available for later infrastructure checkpoints | CLI 29.5.3, Compose 5.1.4, Engine 29.5.3; Linux Docker Desktop daemon reachable. Existing Core MongoDB and observability Compose services are running. Normal sandbox access is denied | PASS — capability confirmed; isolated test fixture still required |

## E2E

| Scenario | Environment | Expected | Actual | Result |
|---|---|---|---|---|
| Docker-backed Redis/PostgreSQL/Python telemetry and lifecycle | Windows Docker Desktop | Live services, lifecycle, and actual telemetry evidence | Not run. Docker is reachable only with elevated access; existing Core/observability stacks are active and no isolated test namespace was established | NOT RUN |
| Native Windows install/service/Collector/gateway verification | Native Windows host | Lifecycle and telemetry acceptance evidence | Not run as a release E2E; unit/integration tests do not substitute | NOT_TESTED |
| Native Linux install/service/Collector/gateway verification | Native Linux host | Lifecycle and telemetry acceptance evidence | Not run; only Linux/amd64 cross-build and Linux-target import view ran | NOT_TESTED |
## Git and preservation state

- Local branches `chore/v0.1.4a-next` and `feat/v0.1.4a-next-work` were created in the isolated worktree from the recorded baseline. Neither has a remote tracking branch. `feat/v0.1.4a-next-work` is active.
- The original `dev` checkout remains at `1a32602` and still has its original staged lock file, modified entrypoints/architecture, historical verification folder, and untracked v0.1.4a-next documents. Its state was rechecked after preparation. No original files were removed or rewritten.
- The shared submodule was not modified. The target worktree and index both record gitlink `c4cd59a9bcaaf0735c3107424f1be98fc498670a`. The policy text was read from the original checkout and is consistent with the local supplement, but Git could not verify the submodule working tree/status; the earlier original-checkout inspection reported submodule dirtiness, so exact content and clean-state assertions remain unverified.
- A 0-byte `cmd/agentctl/otel.yaml.lifecycle.lock` is also present as untracked in the isolated worktree. Its creation/source could not be established; it was left untouched and is excluded from PR-01 changes.
- Only the reviewed PR-01 file set was staged; the unexplained lock artifact was excluded. Local implementation commit `75aaf9a36f95778effb70a06750b743d2b3f562e` is on the feature branch. No push, remote branch, PR, or merge was created.

## Changed-file list

- `.github/workflows/ci.yml`
- `docs/README.md`, `docs/ARCHITECTURE.md`
- `docs/local/architecture/v0.1.4a-next/` (architecture, context boundaries, source baseline, CLI contract, OTel/OpAMP notes, diagrams, integration reconciliation)
- `docs/local/development/coding-policy.md`
- `docs/local/plans/v0.1.4a-next/` (plan index, migration plan, PR-01–PR-10 plans, live release progress)
- `docs/local/requirements/v0.1.4a-next/`
- `docs/local/runbooks/v0.1.4a-next/`
- `docs/local/verification/v0.1.4a-next/` (test matrix, `pr-01-baseline.md`, and required `pr-01-report.md`)
- `internal/agenthealth/doc.go`
- `scripts/check-architecture.ps1`

The isolated worktree also reports the unrelated 0-byte lock artifact noted above; do not stage it.

## Regressions / Security

- No runtime behavior, persistence schema, enrollment, credentials, dependency ownership, or Collector lifecycle changed. The Go addition is package documentation only.
- The new CI check changes build verification only. It adds no runtime dependency or network operation.
- No secrets, endpoints, tokens, or customer data were added to the repository.
- No telemetry-delivery, service lifecycle, or remote-operation security claim is made from the local Go suite.
## Open gates before integration review/publication

1. Restore reliable `docs/shared` submodule metadata/status access and verify the checked-out content against gitlink `c4cd59a9bcaaf0735c3107424f1be98fc498670a`. The policy text currently readable in the original checkout is consistent with the local supplement.
2. Restore valid GitHub CLI authentication and API connectivity; then rerun read-only repository access checks before any push or PR operation.
3. Review the `service` root-help discrepancy and transition allowances at the next appropriate CLI/architecture checkpoint; do not change them silently in PR-01.

The full documentation scan also found four existing historical links outside the release-doc scope: `docs/local/handoffs/2026-10-26-architecture-audit-m1-refactor.md` → `../superpowers/plans/2026-10-26-m1-refactor-plan.md`; `docs/superpowers/plans/2026-10-27-m2-local-installation-identity.md` → `../../architecture/m2-local-installation-identity-design.md` and `../../architecture/target-architecture-v0.1.4.md`; and `docs/superpowers/plans/2026-10-28-m2-core-gateway-endpoint-sync.md` → `../../architecture/m2-core-gateway-endpoint-sync-design.md`. These were not changed as part of PR-01.

Docker Engine access is a separate release gate for PR-05/06/10. The Linux Docker Desktop daemon is currently reachable with elevated read-only access, and existing Core/observability stacks are active. Establish an isolated test namespace before running Docker-backed application telemetry; run native Windows/Linux install, lifecycle, Collector, and delivery scenarios on their real hosts before making release claims.
## Next checkpoint readiness

PR-02 is **BLOCKED** until PR-01 can be reviewed and merged into `chore/v0.1.4a-next`. Before PR-02 code, define the event delivery, transaction/commit ordering, process-crash recovery, duplicate delivery, and SQLite compatibility contracts. No PR-02 implementation or contract decision is included in this checkpoint.

## Rollback

Revert the PR-01 CI trigger/guard, `agenthealth/doc.go`, and v0.1.4a-next documentation changes from the isolated development branch. No runtime data, SQLite schema, Collector installation, dependency state, shared submodule, or remote repository state requires rollback.
