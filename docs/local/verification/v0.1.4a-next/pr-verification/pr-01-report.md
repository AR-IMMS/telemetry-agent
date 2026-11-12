# PR-01 — foundation checkpoint report

**Canonical release:** `v0.1.4a-next`<br>
**Status:** PASS — merged after final Windows and Ubuntu CI passed<br>
**Base commit:** `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`<br>
**Branch / PR / Commit SHA:** `feat/v0.1.4a-next-work` → `chore/v0.1.4a-next` / [PR #20](https://github.com/AR-IMMS/telemetry-agent/pull/20) / final tested PR head `fa9051eb2734a19296c35d9b13c37c18877affa7` / merge commit `7ba6efb86218969f539c17bea7a0a7ea6a972169`<br>
**Integration branch:** `chore/v0.1.4a-next` at merge commit `7ba6efb86218969f539c17bea7a0a7ea6a972169` at checkpoint completion<br>
**Report date:** 2026-11-12

## Summary

PR-01 establishes the reviewed release documentation, preserves current runtime contracts, documents the source baseline, adds a narrow CI architecture/package-documentation/file-size guard, and enables CI for pull requests targeting the approved integration branch. No runtime behavior, SQLite schema, Collector artifact, dependency ownership, enrollment behavior, or shared-submodule content changed. PR #20 was merged after both Windows and Ubuntu CI jobs passed on final head `fa9051eb2734a19296c35d9b13c37c18877affa7`; merge commit is `7ba6efb86218969f539c17bea7a0a7ea6a972169`.

PR-01 completed in the isolated worktree and PR #20 is merged. The user's Windows environment returned `iSenity1812` for `gh api user --jq .login`; `gh auth status`, `gh repo view`, and PR operations succeeded. Both CI jobs passed on the final PR head. The shared gitlink remains unchanged at `c4cd59a9bcaaf0735c3107424f1be98fc498670a`; the original checkout's submodule worktree status could not be established and was left untouched. Docker Engine is available, but no isolated PR-01 infrastructure scenario was defined or run. Native Windows/Linux release E2E remains a later gate.

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
| GitHub Actions PR checks | `https://github.com/AR-IMMS/telemetry-agent/pull/20` | Ubuntu and Windows matrix: manifest probe, architecture guard, tests, vet, build | Both jobs passed for head `0f9096044c558441f73432d3dc5016085c51bdd4`; new report-sync commit requires rerun | PASS for recorded head; final head pending |
| Read shared coding policy | GitHub at pinned target gitlink; original checkout | Read and compare policy | Policy at `c4cd59a9bcaaf0735c3107424f1be98fc498670a` was reviewed and matches the text previously read in the original checkout; it is consistent with the local supplement. Original submodule worktree status remains unverified | PARTIAL |
| GitHub CLI checks | `gh` in user Windows environment | CLI authentication and repo/API access | `gh api user --jq .login` returned `iSenity1812`; `gh auth status` and `gh repo view` succeeded | PASS |
| GitHub PR access | `AR-IMMS/telemetry-agent` | Open PR against approved integration branch | Draft PR #20 created from `feat/v0.1.4a-next-work` to `chore/v0.1.4a-next` | PASS |
| Remote refs | Git protocol + GitHub API | Confirm approved base and resulting branch heads | `dev` and `chore/v0.1.4a-next` at `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`; feature head at `5a0032242a1de8554c5af3c716b0be703cc8382b` | PASS |
| Remote CI / PR review | GitHub Actions / PR #20 | Required checks and review on PR-01 | Ubuntu and Windows checks passed on `0f9096044c558441f73432d3dc5016085c51bdd4`; final report-sync head is pending its CI rerun; PR remains draft | VERIFYING |
| Docker engine probe (release readiness) | Windows host; elevated read-only access | Docker client and engine available for later infrastructure checkpoints | CLI 29.5.3, Compose 5.1.4, Engine 29.5.3; Linux Docker Desktop daemon reachable. Existing Core MongoDB and observability Compose services are running. Normal sandbox access is denied | PASS — capability confirmed; isolated test fixture still required |

## E2E

| Scenario | Environment | Expected | Actual | Result |
|---|---|---|---|---|
| Docker-backed Redis/PostgreSQL/Python telemetry and lifecycle | Windows Docker Desktop | Live services, lifecycle, and actual telemetry evidence | Not run. Docker is reachable only with elevated access; existing Core/observability stacks are active and no isolated test namespace was established | NOT RUN |
| Native Windows install/service/Collector/gateway verification | Native Windows host | Lifecycle and telemetry acceptance evidence | Not run as a release E2E; unit/integration tests do not substitute | NOT_TESTED |
| Native Linux install/service/Collector/gateway verification | Native Linux host | Lifecycle and telemetry acceptance evidence | Not run; only Linux/amd64 cross-build and Linux-target import view ran | NOT_TESTED |
## Git and preservation state

- PR #20 merged `feat/v0.1.4a-next-work` into `chore/v0.1.4a-next` at `7ba6efb86218969f539c17bea7a0a7ea6a972169`. After merge, the reusable feature branch was recreated from the integration branch and pushed without force; PR-02 work starts from that merge commit.
- The original `dev` checkout remains at `1a32602` and still has its original staged lock file, modified entrypoints/architecture, historical verification folder, and untracked v0.1.4a-next documents. Its state was rechecked after preparation. No original files were removed or rewritten.
- The shared submodule was not modified. The target worktree and index both record gitlink `c4cd59a9bcaaf0735c3107424f1be98fc498670a`. The policy text was read from the original checkout and is consistent with the local supplement, but Git could not verify the submodule working tree/status; the earlier original-checkout inspection reported submodule dirtiness, so exact content and clean-state assertions remain unverified.
- A 0-byte `cmd/agentctl/otel.yaml.lifecycle.lock` is also present as untracked in the isolated worktree. Its creation/source could not be established; it was left untouched and is excluded from PR-01 changes.
- Only reviewed PR-01 files were staged; the unexplained lock artifact was excluded. PR #20 was merged; no force push was used. The reusable feature branch was recreated at the merge commit.

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

1. Keep the shared submodule untouched; its original checkout's local status remains unverifiable. Recheck only if a later task requires changing or depending on its local content.
2. PR-02 is complete: see [`pr-02-report.md`](pr-02-report.md). Continue with PR-03 contracts on the recreated reusable feature branch.
3. Review the `service` root-help discrepancy and transition allowances at the next appropriate CLI/architecture checkpoint; do not change them silently.

The full documentation scan also found four existing historical links outside the release-doc scope: `docs/local/handoffs/2026-10-26-architecture-audit-m1-refactor.md` → `../superpowers/plans/2026-10-26-m1-refactor-plan.md`; `docs/superpowers/plans/2026-10-27-m2-local-installation-identity.md` → `../../architecture/m2-local-installation-identity-design.md` and `../../architecture/target-architecture-v0.1.4.md`; and `docs/superpowers/plans/2026-10-28-m2-core-gateway-endpoint-sync.md` → `../../architecture/m2-core-gateway-endpoint-sync-design.md`. These were not changed as part of PR-01.

Docker Engine access is a separate release gate for PR-05/06/10. The Linux Docker Desktop daemon is currently reachable with elevated read-only access, and existing Core/observability stacks are active. Establish an isolated test namespace before running Docker-backed application telemetry; run native Windows/Linux install, lifecycle, Collector, and delivery scenarios on their real hosts before making release claims.
## Next checkpoint readiness

PR-02 is **PASS** and merged as PR #21 at `d71f5aa39777824e29f1d06a364bb0a289373803`; see [`pr-02-report.md`](pr-02-report.md). PR-03 is the next checkpoint.

## Rollback

Revert the PR-01 CI trigger/guard, `agenthealth/doc.go`, and v0.1.4a-next documentation changes from the isolated development branch. No runtime data, SQLite schema, Collector installation, dependency state, shared submodule, or remote repository state requires rollback.
