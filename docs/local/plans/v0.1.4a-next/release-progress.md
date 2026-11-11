# v0.1.4a-next — release progress

**Canonical target:** `v0.1.4a-next`<br>
**Execution state:** BLOCKED — PR-01 implementation and local verification are complete; shared-policy/submodule and GitHub-auth/API gates prevent integration. Docker Engine access blocks later infrastructure checkpoints, not PR-01. No runtime-behavior changes made.<br>
**Checkpoint rule:** PR-01–PR-10 are verified checkpoints; they do not require ten separate GitHub PRs. Reconcile the GitHub PR shape with the approved integration branch before publishing.<br>
**Last updated:** 2026-11-12

## Preservation invariants

- The original `dev` checkout at `D:\Study\FPTU\FPTU\WDP301\projects\telemetry-agent` remains untouched by this preparation. Its staged, unstaged, untracked, and submodule state is preserved as found.
- Work is isolated in `C:\Users\pvduc\.codex\worktrees\v014a-next-pr01\telemetry-agent`. The local integration branch `chore/v0.1.4a-next` starts at baseline `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`; reusable development branch `feat/v0.1.4a-next-work` is the active branch.
- `docs/shared/` is a Git submodule and is not modified. The checked-out target worktree does not expose its policy file; submodule state and shared policy remain unresolved access gates.
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
| Shared policy and submodule | BLOCKED | `docs/shared/engineering/AI_CODING_POLICY.md` is unavailable in the worktree; do not claim cross-policy validation. |
| GitHub access | BLOCKED | Existing `gh` reports invalid `GH_TOKEN` and saved token; API requests failed to connect. No remote refs, branches or PRs changed. |
| Docker infrastructure | BLOCKED | Docker CLI 29.5.3 / Compose 5.1.4 are installed, but the Docker Engine named pipe is inaccessible. No infrastructure E2E was run. |
| Native Linux E2E | NOT RUN | A Windows-hosted cross-build is not native Linux execution. |

## PR-01 contracts and gates

Define and verify the following before any dependent change:

1. **CLI compatibility:** preserve current command names, supported flags, output/exit semantics, `--state-path`, revision/CAS behavior, and integration/template workflows. Record real command help and regression evidence. The current help discrepancy for Windows-only `service` is documented for a later CLI checkpoint; do not silently change it in PR-01.
2. **SQLite compatibility:** keep existing migration ledger/schema and persisted state stable. Current source includes a migration ledger (tests assert version 13); no table/schema or migration change in PR-01. Preserve old fixture/reopen behavior for later storage work.
3. **Collector and dependencies:** retain the pinned Collector manifest (`otelcol-contrib` 0.160.0 with per-artifact SHA256), existing receiver/dependency set, ownership-safe lifecycle, activation journal, and LKG recovery. No artifact, generated config, service, enrollment, or dependency behavior change.
4. **Architecture ownership:** document current package responsibilities and dependency directions from source, not just the proposed tree. The documented tree remains a flexible baseline. Add no mechanical package moves.
5. **Coding-policy agreement:** inspect the shared policy and resolve any real conflict before adopting/enforcing a new rule. The repository supplement's 300–400 typical-file guidance plus 450 hard ceiling is not independently authoritative while the shared submodule is unavailable.
6. **Platform verification:** local Windows tests/build and Linux cross-build are distinct from native Windows/Linux service, Docker, Collector, and gateway E2E. Record blocked gates honestly.

Unresolved Core config authority, OpAMP identity/status mapping, remote-operation authorization, and shared contract publication are not PR-01 decisions. Do not invent those contracts; resolve them before PR-07/PR-09 or their first dependent checkpoint.

## Checkpoints

| Checkpoint | Scope | Dependencies | Status | Verification / blockers | PR and commit references |
|---|---|---|---|---|---|
| PR-01 Foundation | Documentation, source ownership baseline, package docs, architecture guard, CI target | None | BLOCKED | Local Windows tests/vet/build and Linux cross-build pass; architecture guard passes for 23 packages; 0 broken local links. Blocked on shared submodule/policy access and invalid GH token/API connectivity. | `feat/v0.1.4a-next-work` → `chore/v0.1.4a-next`; no PR; commit `75aaf9a36f95778effb70a06750b743d2b3f562e`; base `1a32602` |
| PR-02 Events & SQLite | Typed event delivery and technical persistence layer | PR-01 merged; commit/failure/transaction contracts before code | BLOCKED | Not started. Blocked by PR-01 integration gate. | None |
| PR-03 App composition | Thin app host and CLI adapters | PR-02 merged; CLI and use-case contracts | BLOCKED | Not started. Blocked by preceding checkpoint. | None |
| PR-04 Context migration | Incremental repository/owner migration | PR-03 merged; per-context contracts before each slice | BLOCKED | Not started. Blocked by preceding checkpoint. | None |
| PR-05 OTel discovery | Pinned-binary Observer/Receiver Creator spike and gated transfer | PR-04 merged; platform capability/parity evidence; one receiver owner | BLOCKED | Not started. Docker and native Linux evidence unavailable; blocked by preceding checkpoint and infrastructure gates. | None |
| PR-06 Monitoring profiles | Redis/PostgreSQL/Python profiles and mock facilities | PR-05 parity; profile/config authority contract | BLOCKED | Not started. Docker Engine unavailable; blocked by preceding checkpoint and authority decisions. | None |
| PR-07 OpAMP | Core/Agent configuration and status integration | PR-04 contracts; jointly approved Core authority, identity, status, and error mapping | BLOCKED | Not started. Core authority and shared contract publication remain unresolved; blocked by preceding checkpoint. | None |
| PR-08 Health & reporting | Agent diagnostics and truthful health/evidence reporting | PR-07 status/reporting contracts | BLOCKED | Not started. Blocked by preceding checkpoint and measurable evidence contract. | None |
| PR-09 Controlled operations | Bounded diagnostic operations and security-gated mutations | PR-03; approved authorization/idempotency/audit/target contract | BLOCKED | Not started. Security policy is unresolved; no mutation contract may be invented. | None |
| PR-10 Release hardening | Native install/upgrade/uninstall and full release matrix | PR-01–PR-09 verified | BLOCKED | Not started. Docker, native Linux, GitHub and shared-policy gates remain open. | None |

Update this file after each meaningful milestone and checkpoint. Use only `TODO`, `IN_PROGRESS`, `VERIFYING`, `PASS`, `FAIL`, or `BLOCKED`. Every checkpoint report must be named `docs/local/verification/v0.1.4a-next/pr-verification/pr-XX-report.md` and record branch, PR/commit SHA, tests, E2E, security/regressions, limitations, documentation, rollback, and next-checkpoint readiness. Do not mark a checkpoint PASS while a required gate is BLOCKED or NOT TESTED.
