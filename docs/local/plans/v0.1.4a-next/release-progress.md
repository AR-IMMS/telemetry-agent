# v0.1.4a-next — release progress

**Canonical target:** `v0.1.4a-next`<br>
**Execution state:** IN_PROGRESS — PR-01 is open as draft #20 against `chore/v0.1.4a-next`; both Windows and Ubuntu CI jobs passed for head `0f9096044c558441f73432d3dc5016085c51bdd4`. The checkpoint report is being synchronized with the PR state before final-head CI and merge. User-environment `gh` authentication is verified. The shared gitlink is unchanged and its pinned commit exists. Docker Engine 29.5.3 / Compose 5.1.4 are available with elevated access; no isolated test namespace has been established for infrastructure tests. No runtime-behavior changes made.<br>
**Checkpoint rule:** PR-01–PR-10 are verified checkpoints; they do not require ten separate GitHub PRs. Keep each submitted checkpoint targeted at the approved integration branch.<br>
**Last updated:** 2026-11-12

## Preservation invariants

- The original `dev` checkout at `D:\Study\FPTU\FPTU\WDP301\projects\telemetry-agent` remains untouched by this preparation. Its staged, unstaged, untracked, and submodule state is preserved as found.
- Work is isolated in `C:\Users\pvduc\.codex\worktrees\v014a-next-pr01\telemetry-agent`. The local integration branch `chore/v0.1.4a-next` starts at baseline `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`; reusable development branch `feat/v0.1.4a-next-work` is the active branch.
- `docs/shared/` is a Git submodule and is not modified. The target/index gitlink is `c4cd59a9bcaaf0735c3107424f1be98fc498670a`; the pinned policy file was reviewed through GitHub and matches the policy text previously read in the original checkout. The original checkout's local submodule worktree status remains unverifiable and untouched.
- Do not use or modify the unrelated staged `cmd/agentctl/otel.yaml.lifecycle.lock`, `.local-verification/`, `docs/local/architecture/target-architecture-v0.1.4.md`, `docs/local/architecture/architecture-refactoring-proposal-phase-1.md`, or `docs/local/architecture/telemetry-agent-template-autodiscovery-goal-en.md` from the original checkout. A 0-byte same-name lock file appeared as untracked in this worktree after branch setup; it is left untouched and excluded from PR-01.
- Existing CLI behavior, SQLite schema/data, Collector artifacts, dependency ownership, enrollment, activation and last-known-good behavior are compatibility constraints. No runtime change is part of PR-01.

## Baseline evidence

| Check | Result | Notes |
|---|---|---|
| Git baseline | PASS | Isolated worktree clean at `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`; both local branch names were absent before creation. |
| Original checkout preservation | PASS / ongoing | Existing original dirty state was inspected and left in place. Its `git status` cannot traverse `.git/modules/docs/shared`; parent status with submodules ignored showed the staged/unstaged/untracked files recorded above. |
| `go test ./...` | PASS | All Go packages passed on Windows after a sandboxed run exposed temp-file/SQLite access-denied failures; the authorized normal-temp run passed. |
| `go vet ./...` | PASS | Baseline on Windows. |
| `go build ./...` | PASS | Baseline on Windows. |
| Linux/amd64 cross-build | PASS | `GOOS=linux GOARCH=amd64 go build ./...` from Windows. This is a build check, not Linux E2E. |
| CLI compatibility snapshot | PASS | Actual root `agentctl help` output and exit code recorded in [PR-01 baseline](../../verification/v0.1.4a-next/pr-verification/pr-01-baseline.md). `agentctl --help` currently exits 2 with unknown command; `service` is implemented but absent from root help. |
| Shared policy and submodule | PARTIAL | Policy at pinned gitlink `c4cd59a9bcaaf0735c3107424f1be98fc498670a` was reviewed and is consistent with the local supplement. The original checkout's worktree status remains unverifiable; no shared files or gitlink changed. |
| GitHub access | PASS | User-environment `gh api user --jq .login` returned `iSenity1812`; `gh auth status`, `gh repo view`, and PR creation succeeded. |
| Docker infrastructure | PARTIAL | Docker CLI 29.5.3 / Compose 5.1.4 and Engine 29.5.3 are available using elevated read-only access; existing Core and observability services are running. No isolated fixture/E2E was run. |
| Native Linux E2E | NOT RUN | A Windows-hosted cross-build is not native Linux execution. |

## PR-01 contracts and gates

Define and verify the following before any dependent change:

1. **CLI compatibility:** preserve current command names, supported flags, output/exit semantics, `--state-path`, revision/CAS behavior, and integration/template workflows. Record real command help and regression evidence. The current help discrepancy for Windows-only `service` is documented for a later CLI checkpoint; do not silently change it in PR-01.
2. **SQLite compatibility:** keep existing migration ledger/schema and persisted state stable. Current source includes a migration ledger (tests assert version 13); no table/schema or migration change in PR-01. Preserve old fixture/reopen behavior for later storage work.
3. **Collector and dependencies:** retain the pinned Collector manifest (`otelcol-contrib` 0.160.0 with per-artifact SHA256), existing receiver/dependency set, ownership-safe lifecycle, activation journal, and LKG recovery. No artifact, generated config, service, enrollment, or dependency behavior change.
4. **Architecture ownership:** document current package responsibilities and dependency directions from source, not just the proposed tree. The documented tree remains a flexible baseline. Add no mechanical package moves.
5. **Coding-policy agreement:** the shared policy at the pinned gitlink was reviewed. Its 300–400-line guidance and justification for larger files are consistent with the local supplement's 450-line guard and documented transition exception; no policy conflict was found.
6. **Platform verification:** local Windows tests/build and Linux cross-build are distinct from native Windows/Linux service, Docker, Collector, and gateway E2E. Record blocked gates honestly.

Unresolved Core config authority, OpAMP identity/status mapping, remote-operation authorization, and shared contract publication are not PR-01 decisions. Do not invent those contracts; resolve them before PR-07/PR-09 or their first dependent checkpoint.

## Checkpoints

| Checkpoint | Scope | Dependencies | Status | Verification / blockers | PR and commit references |
|---|---|---|---|---|---|
| PR-01 Foundation | Documentation, source ownership baseline, package docs, architecture guard, CI target | None | VERIFYING | Local Windows tests/vet/build and Linux cross-build pass; architecture guard passes for 23 packages; PR #20's Ubuntu and Windows jobs passed on head `0f9096044c558441f73432d3dc5016085c51bdd4`. Report synchronization will update the PR head and rerun CI before merge. | [PR #20](https://github.com/AR-IMMS/telemetry-agent/pull/20), `feat/v0.1.4a-next-work` → `chore/v0.1.4a-next`; verified head `0f9096044c558441f73432d3dc5016085c51bdd4`; base `1a32602` |
| PR-02 Events & SQLite | Typed event delivery and technical persistence layer | PR-01 merged; commit/failure/transaction contracts before code | BLOCKED | Not started. Blocked by PR-01 integration gate. | None |
| PR-03 App composition | Thin app host and CLI adapters | PR-02 merged; CLI and use-case contracts | BLOCKED | Not started. Blocked by preceding checkpoint. | None |
| PR-04 Context migration | Incremental repository/owner migration | PR-03 merged; per-context contracts before each slice | BLOCKED | Not started. Blocked by preceding checkpoint. | None |
| PR-05 OTel discovery | Pinned-binary Observer/Receiver Creator spike and gated transfer | PR-04 merged; platform capability/parity evidence; one receiver owner | BLOCKED | Not started. Docker Engine is available with elevated access; an isolated fixture and native Linux evidence remain required. Blocked by preceding checkpoint. | None |
| PR-06 Monitoring profiles | Redis/PostgreSQL/Python profiles and mock facilities | PR-05 parity; profile/config authority contract | BLOCKED | Not started. Docker is available; build isolated fixtures after PR-05 and resolve the profile/config authority contract. Blocked by preceding checkpoint and authority decision. | None |
| PR-07 OpAMP | Core/Agent configuration and status integration | PR-04 contracts; jointly approved Core authority, identity, status, and error mapping | BLOCKED | Not started. Core authority and shared contract publication remain unresolved; blocked by preceding checkpoint. | None |
| PR-08 Health & reporting | Agent diagnostics and truthful health/evidence reporting | PR-07 status/reporting contracts | BLOCKED | Not started. Blocked by preceding checkpoint and measurable evidence contract. | None |
| PR-09 Controlled operations | Bounded diagnostic operations and security-gated mutations | PR-03; approved authorization/idempotency/audit/target contract | BLOCKED | Not started. Security policy is unresolved; no mutation contract may be invented. | None |
| PR-10 Release hardening | Native install/upgrade/uninstall and full release matrix | PR-01–PR-09 verified | BLOCKED | Not started. Native Linux E2E and all preceding checkpoint gates remain open; Docker and GitHub access are available. | None |

Update this file after each meaningful milestone and checkpoint. Use only `TODO`, `IN_PROGRESS`, `VERIFYING`, `PASS`, `FAIL`, or `BLOCKED`. Every checkpoint report must be named `docs/local/verification/v0.1.4a-next/pr-verification/pr-XX-report.md` and record branch, PR/commit SHA, tests, E2E, security/regressions, limitations, documentation, rollback, and next-checkpoint readiness. Do not mark a checkpoint PASS while a required gate is BLOCKED or NOT TESTED.
