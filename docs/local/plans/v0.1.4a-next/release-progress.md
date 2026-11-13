# v0.1.4a-next — release progress

**Canonical target:** `v0.1.4a-next`<br>
**Execution state:** IN_PROGRESS — PR-01 (#20) through PR-06 are merged to `chore/v0.1.4a-next`. PR-07 remains IN_PROGRESS in draft PR #30 on `feat/pr-07-opamp`. Exact docs-updated head `d7a123a4692912fb66eb5976a7b99f3f28e21e7d` passed Windows and Ubuntu CI run [43451862541](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43451862541), including full tests, managed-runtime E2E, native service migration/restart/rollback/uninstall, vet, and build. Ubuntu also passed isolated Docker recovery and Agent-managed Redis/PostgreSQL/Python Prometheus/mock-cooling profile E2Es. Hosted Windows Docker/profile steps were skipped; Windows Agent-managed Docker-profile parity remains open, and the local Windows managed-owner attempt is blocked at protected credential setup because this process cannot obtain an elevated token. PR-07 report includes evidence and exact environment limits. Supervisor remains opt-in; existing installations remain unchanged pending explicit migration. Production NestJS Core interoperability is tracked separately.<br>
**Checkpoint rule:** PR-01–PR-10 are verified checkpoints; they do not require ten separate GitHub PRs. Keep each submitted checkpoint targeted at the approved integration branch.<br>
**Last updated:** 2026-11-13

PR-07 follow-up: a test-only managed-runtime failure injection now exercises
Collector crash replacement, Supervisor crash process-tree cleanup, and Agent
runtime restart with OTLP recovery. Native Windows full tests, vet, build, and
the architecture guard passed. CI run `43419406085` ran the E2E on both OSes but
exposed a test selector assumption: the Agent launches a snapshot config under
`.collector-launch-*`, not the stable path. The fixture now uses a unique
runtime temp root and puts sink Collectors outside it; the failure remains
unverified pending exact-head rerun. The opt-in live test could not start
locally because this Windows token lacks protected credential-store access.
PR #30 remains draft and Supervisor remains opt-in.

## PR-07c final-head CI update — 2026-11-13

- The documentation-updated PR head `e159c9e63c0c525bf37d61bcc336ba481e3625d6`
  completed Windows and Ubuntu CI in run
  [`43429611905`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43429611905),
  attempt 2. Both full test, managed runtime/control E2E, vet, and build jobs
  passed. Linux Docker and application-profile E2E passed. The first attempt
  timed out waiting for post-restart OTLP telemetry after policy revision 4
  had been applied; retrying the failed Linux job on the same head passed. Keep
  the intermittent timeout visible as a test-stability issue.
- Local `go test ./internal/agentlifecycle -count=1` passed when `TEMP` and
  `TMP` pointed to an isolated directory under the writable workspace. The
  default sandbox temp path produced SQLite `unable to open database file`
  failures; this was an environment limitation, not a code change. Local Docker
  access is denied on `dockerDesktopLinuxEngine`, and the current Windows token
  has the Administrators group deny-only. Native service installation or
  removal was not attempted on the developer host.
- The open gates remain native Windows/Linux service install, migration,
  upgrade, restart, and uninstall, plus Windows Docker profile parity. Hosted
  Windows CI skips the Docker profile tests, so Linux Docker E2E does not close
  Windows parity. PR #30 stays draft, Supervisor stays opt-in, and PR-07 stays
  IN_PROGRESS. No merge, ownership transfer, or PR-08 start is authorized by
  these results.

## Preservation invariants

- The original `dev` checkout at `D:\Study\FPTU\FPTU\WDP301\projects\telemetry-agent` remains untouched by this preparation. Its staged, unstaged, untracked, and submodule state is preserved as found.
- Work is isolated in `C:\Users\pvduc\.codex\worktrees\v014a-next-pr01\telemetry-agent`. The local integration branch `chore/v0.1.4a-next` started at baseline `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`; reusable development branch `feat/v0.1.4a-next-work` was used for earlier checkpoints. Current branch `feat/pr-07-opamp` is based on PR-06 merge `af2f5db4c6b8832bc7bcc87d332d899dad01b452`.
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

Unresolved Core config authority, OpAMP identity/status mapping, remote-operation authorization, and shared contract publication are not PR-01 decisions. Do not invent those contracts; resolve them before their dependent PR-07b/07c or PR-09 checkpoint. PR-07a capability verification is independent and does not authorize runtime ownership transfer.

## Checkpoints

| Checkpoint | Scope | Dependencies | Status | Verification / blockers | PR and commit references |
|---|---|---|---|---|---|
| PR-01 Foundation | Documentation, source ownership baseline, package docs, architecture guard, CI target | None | PASS | Local Windows tests/vet/build and Linux cross-build pass; architecture guard passes for 23 packages. Final Windows and Ubuntu CI passed on PR head `fa9051eb2734a19296c35d9b13c37c18877affa7`; merged as `7ba6efb86218969f539c17bea7a0a7ea6a972169`. Native platform E2E remains a later release gate. | [PR #20](https://github.com/AR-IMMS/telemetry-agent/pull/20), `feat/v0.1.4a-next-work` → `chore/v0.1.4a-next`; merge `7ba6efb86218969f539c17bea7a0a7ea6a972169` |
| PR-02 Events & SQLite | Typed event delivery and technical persistence layer | PR-01 merged; commit/failure/transaction contracts before code | PASS | Local Windows/Linux cross-build gates passed. Final Windows and Ubuntu CI, including exact Collector artifact probes, passed on `21958ac`; PR #21 merged at `d71f5aa`. Native platform E2E remains a release-level gate. | [PR #21](https://github.com/AR-IMMS/telemetry-agent/pull/21), merge `d71f5aa39777824e29f1d06a364bb0a289373803` |
| PR-03 App composition | Thin app host and CLI adapters | PR-02 merged; CLI and use-case contracts | PASS | Full Windows suite, focused race, vet/build, Linux cross-build, architecture guard, final-head Windows/Ubuntu CI, and isolated live Windows runtime E2E passed. Native Linux E2E remains a release-level gate. See [PR-03 report](../../verification/v0.1.4a-next/pr-verification/pr-03-report.md). | [PR #22](https://github.com/AR-IMMS/telemetry-agent/pull/22); implementation `3f88d77b68c9d18bec819d3822733fa535052ffc`; merge `eaaedc8ef229113573903bce9fa72c608025628d`; CI run `43138456859` |
| PR-04 Context migration | Incremental repository/owner migration | PR-03 merged; per-context contracts before each slice | PASS | PR-04a identity (#23), PR-04b enrollment (#24), PR-04c inventory/proposal (#25), PR-04d Configuration (#26), and PR-04e reconciliation runtime apply ports (#27) are merged; PR-04e merge SHA is `beb95aa`. It passed full Windows tests, focused race, vet/build, Linux/amd64 cross-build, architecture guard, live Windows Docker/Collector runtime E2E, independent review, and final-head Windows/Ubuntu CI run `43178655519`. The optional platform dependency adapter extraction is deferred: no bounded contract or release need justifies changing its existing build-tagged operations; see the PR-04 ruling. | [PR-04 plan](pr-04-context-migration.md); [PR-04a report](../../verification/v0.1.4a-next/pr-verification/pr-04a-report.md); [PR-04b report](../../verification/v0.1.4a-next/pr-verification/pr-04b-report.md); [PR-04c report](../../verification/v0.1.4a-next/pr-verification/pr-04c-report.md); [PR-04d report](../../verification/v0.1.4a-next/pr-verification/pr-04d-report.md); [PR-04e report](../../verification/v0.1.4a-next/pr-verification/pr-04e-report.md) |
| PR-05 OTel discovery | Pinned-binary Observer/Receiver Creator with Docker/Redis profile and single-owner transfer | PR-04 merged; approved identity/opt-in/endpoint/ownership contracts; Windows/Linux parity | PASS | Draft [PR #28](https://github.com/AR-IMMS/telemetry-agent/pull/28) merged as `11fdd1e720c542898ba9c0c673063b1d698d7451`. Final-head CI run [43261742475](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43261742475) passed Windows and Ubuntu; Ubuntu covered isolated Docker outage/recovery with inventory retention, receiver convergence and no generation change, live `endpoint_unreachable`, full tests, vet, and build. Windows full tests, live Docker/Collector/OTLP, vet, and build passed. Local focused gates include pinned profile/LKG failures, Linux cross-build, SQLite polling `-count=100`, and Windows target-port 6380 live delivery. |
| PR-06 Monitoring profiles | Redis/PostgreSQL/Python profiles and mock facilities | PR-05 parity; local profile schema and credential-reference contract before dependent implementation | PASS | [PR #29](https://github.com/AR-IMMS/telemetry-agent/pull/29) merged into `chore/v0.1.4a-next` as `af2f5db4c6b8832bc7bcc87d332d899dad01b452`. Final documentation-updated head CI [43275105064](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43275105064) passed Windows and Ubuntu. The corrected exact implementation head also passed isolated Linux Docker recovery and Redis/PostgreSQL/Python/mock-profile OTLP E2E. See [PR-06 report](../../verification/v0.1.4a-next/pr-verification/pr-06-report.md) and [Windows live evidence](../../verification/v0.1.4a-next/pr-verification/pr-06-windows-live-output.txt). | [PR #29](https://github.com/AR-IMMS/telemetry-agent/pull/29), merge `af2f5db` |
| PR-07 OpAMP & Supervisor | 07a capability spike; 07b versioned contracts + Go OpAMP server/Mock Core; 07c parity-gated lifecycle adoption | PR-05/06 compatibility; pinned Windows/Linux artifacts and capability evidence; reviewed enrollment/Core baselines | IN_PROGRESS | Exact-head CI run [43450425177](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43450425177) passed Windows and Ubuntu full tests, managed-runtime checks, vet/build, and native Windows/Linux service install, PR-06-to-managed migration, restart, explicit legacy rollback, restart, and uninstall. Service evidence artifacts: Windows `14051765071`, Linux `14050646612`. Ubuntu also passed isolated Docker recovery and Agent-managed Redis/PostgreSQL/Python Prometheus/mock-cooling parity with churn and duplicate-sample checks. Hosted Windows Docker steps are skipped, and local Windows managed-profile parity remains unverified because protected credential setup requires elevation. Still open: Windows Agent-managed Docker-profile parity. PR #30 remains draft; Supervisor remains opt-in and existing installs stay legacy-owned. Production Core interoperability remains separate. See the [PR-07 report](../../verification/v0.1.4a-next/pr-verification/pr-07-report.md). | [Draft PR #30](https://github.com/AR-IMMS/telemetry-agent/pull/30) |
| PR-08 Health & reporting | Agent diagnostics and truthful health/evidence reporting | PR-07 status/reporting contracts | BLOCKED | Not started. Blocked by preceding checkpoint and measurable evidence contract. | None |
| PR-09 Controlled operations | Bounded diagnostic operations and security-gated mutations | PR-03; approved authorization/idempotency/audit/target contract | BLOCKED | Not started. Security policy is unresolved; no mutation contract may be invented. | None |
| PR-10 Release hardening | Native install/upgrade/uninstall and full release matrix | PR-01–PR-09 verified | BLOCKED | Not started. Native Linux E2E and all preceding checkpoint gates remain open; Docker and GitHub access are available. | None |

Update this file after each meaningful milestone and checkpoint. Use only `TODO`, `IN_PROGRESS`, `VERIFYING`, `PASS`, `FAIL`, or `BLOCKED`. Every checkpoint report must be named `docs/local/verification/v0.1.4a-next/pr-verification/pr-XX-report.md` and record branch, PR/commit SHA, tests, E2E, security/regressions, limitations, documentation, rollback, and next-checkpoint readiness. Do not mark a checkpoint PASS while a required gate is BLOCKED or NOT TESTED.

PR-07 follow-up: Windows run [43413389730](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43413389730) failed the full test step at `TestPeriodicSQLitePollingDetectsCommittedCrossProcessDesiredChange` after its one-second startup deadline; a transient SQLite snapshot warning preceded the timeout. Commit `78981ea` changes only that test's deadline to three seconds and defers cancellation. It passed 100 consecutive native Windows runs, and exact-head CI [43415093522](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43415093522) passed on Windows and Ubuntu. No production code changed. PR-07 stays IN_PROGRESS and PR #30 stays draft pending the remaining adoption gates.

PR-07c follow-up: exact-head run [43428314136](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43428314136) on `eb188d396969b564f3576e84a1c5f500cfba41e5` passed the expanded managed-runtime E2E on Windows and Ubuntu. Evidence includes local config update/no-op, active/LKG equivalence, Collector and Supervisor crash recovery, Agent runtime restart, authenticated credential rotation/revocation, durable revision/hash rollback, and OTLP delivery. Ubuntu Docker recovery and all four application profiles passed with churn and duplicate-sample checks; hosted Windows Docker steps were skipped. Native service install/migration/upgrade/uninstall, Windows Docker profile parity, and service restart remain open, so PR-07 remains IN_PROGRESS and Supervisor stays opt-in.

PR-07c final-head update: `e97f454c9179ec5deb83f1deeb80d4ac2a935e80` passed exact-head Windows/Ubuntu CI run [43434645674](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43434645674). The local Windows legacy-owner live profile test passed with actual Docker/Collector/OTLP. The corresponding managed Supervisor profile run was attempted but could not create its protected credential config because the process token is not elevated; UAC elevation was denied by the environment. This is a setup blocker, not a runtime failure. Native service lifecycle gates remain unverified, so PR-07 stays IN_PROGRESS, PR #30 stays draft, and Supervisor stays opt-in.

PR-07c native service update: run [43448710761](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43448710761) passed the full Ubuntu job, including the native Linux install/migration/rollback/restart/uninstall sequence; sanitized native-service artifact `14048169509` is retained. Windows executable migration also succeeded, but its service restart helper attempted `sc start` while state was `STOP_PENDING` (error 1056). Windows evidence artifact `14049576920` is retained. The test now waits for explicit `STOPPED`, and the native Windows service stopper waits through pending SCM states. Exact-head rerun is required. PR-07 remains IN_PROGRESS and PR #30 remains draft.

PR-07c native service exact-head pass: run [43450425177](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43450425177) passed Windows and Ubuntu, including native install/migration/restart/rollback/restart/uninstall. Sanitized service artifacts: Windows `14051765071`, Linux `14050646612`. Windows Agent-managed Docker-profile parity remains open because the hosted Windows job skips Docker and the local protected-credential setup requires elevation. PR-07 remains IN_PROGRESS, PR #30 remains draft, and Supervisor remains opt-in.

PR-07c docs-updated final-head CI: exact head `d7a123a4692912fb66eb5976a7b99f3f28e21e7d` passed Windows and Ubuntu run [43451862541](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43451862541). Both jobs executed full tests, managed-runtime E2E, native service lifecycle E2E, vet, and build. Ubuntu also executed Docker recovery and Agent-managed Redis, PostgreSQL, Python/Prometheus, and mock-cooling parity. Sanitized artifacts: managed Windows `14051522250`, managed Linux `14051917147`, native service Windows `14049933814`, native service Linux `14051876355`, and Ubuntu profile parity `14052975215`. Windows Agent-managed Docker-profile parity remains a mandatory open gate; hosted Windows CI skips Docker and the local managed-owner setup lacks elevation. PR-07 remains IN_PROGRESS, PR #30 remains draft, Supervisor stays opt-in, existing installations stay legacy-owned, and production NestJS Core interoperability remains separate.
