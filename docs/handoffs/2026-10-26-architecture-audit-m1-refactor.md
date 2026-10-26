# Handoff: Full Codebase Architecture Audit and M1 Refactor

Date: 2026-10-26

## Purpose and current state

Continue from the user's annotation “Full Codebase Architecture Audit — main.” The audit was read-only and performed on `main`; the user reviewed it, accepted most findings, narrowed the scope, and then authorized implementation. The implementation is now on a feature branch; do not assume the current branch is `main`.

- Repository: `D:\Code\Projects\telemetry-agent`
- Branch: `feat/m1-refactor-foundation`
- Latest commit: `0f4f8b6` (`feat: add M1 reconciliation and recovery foundation`)
- PR: [AR-IMMS/telemetry-agent#11](https://github.com/AR-IMMS/telemetry-agent/pull/11), base `dev`, head `feat/m1-refactor-foundation`.
- PR #11 was verified OPEN after creation. The most recent status query failed with `error connecting to api.github.com`; recheck PR and CI status when connectivity is available.

## Audit conclusions and user scope decisions

The source audit identified oversized CLI entry/command files, lifecycle orchestration and persistence coupling, state consistency risks around external I/O, missing reconciliation/recovery behavior, and needed cross-platform regression coverage. It found no basis for a rewrite or removal. Existing config rendering, Collector supervision, platform exporters, and much of the current functionality were considered reusable.

The user specifically narrowed the implementation:

- Split only CLI composition and M1 lifecycle/configuration workflows; leave unrelated commands behaviorally unchanged.
- Keep reconciliation narrow in M1: current persisted desired state and managed dependencies. Add discovered resources and integration instances in M3.
- Preserve CLI, Collector, host telemetry, managed exporter behavior, ownership, and Windows/Linux support.
- Deliver M1 through S1 (boundaries), S2 (reconciliation/activation), and S3 (SQLite/recovery); avoid a big-bang rewrite or premature integration SDK.

The full audit and user review were pasted as attachments on the original computer. Their attachment paths were:

- `C:\Users\PC\.codex\attachments\43148438-fba5-4f72-a5d1-45fcc967ff86\Pasted text.txt`
- `C:\Users\PC\.codex\attachments\2c7d9286-f31a-488f-b4ed-9410f3d71e40\Pasted text.txt`

These local paths may not exist on another computer. The decisions above and implementation plan below preserve the portable context; ask the user to reattach the source review if exact wording is needed.

## Implementation and review history

The refactor plan is [docs/superpowers/plans/2026-10-26-m1-refactor-plan.md](../superpowers/plans/2026-10-26-m1-refactor-plan.md). PR #11 extracts agentctl composition/lifecycle workflows; adds a repository seam, SQLite WAL state/migration, revision-bound plans, activation journal and restart recovery; and updates architecture/runbook documentation.

A final code review identified four lifecycle hazards: promotion could precede a failed state commit without startup recovery; readiness could copy newer bytes into last-known-good; process-local activation serialization could permit stale rollback across processes; and teardown could race a dependency re-enable. The implementation addresses these with:

- Durable activation intent and a pre-promotion recovery artifact, restored on commit failure or runtime startup.
- Launch-specific immutable config snapshots; readiness stores the bytes from the config Collector launched.
- A cross-process OS lifecycle lock around config promotion/rollback and managed-resource teardown/re-enable/install side effects. SQLite transactions remain short and state-only. The user explicitly approved this lock approach, which revises the plan's earlier “no lock during external I/O” rule.
- Regression coverage for activation recovery, exact launch snapshot/LKG bytes, and teardown/re-enable serialization.

## Verification at handoff

Passed on Windows: `go test ./... -count=1`, `go vet ./...`, and `go build ./...`. Linux amd64 and Windows amd64 cross-builds passed. Changed production Go files are all at or below 450 LOC. Native Linux test execution is still expected from CI; do not claim Linux runtime tests passed based on cross-compilation.

## Workspace cautions

The commit intentionally contains only M1 work. The workspace also had unrelated modified files under `internal/agenthealth`, `internal/config`, `internal/identity`, and `internal/supervisor`, plus untracked `.agents/`, `.scratch/`, architecture/roadmap documents, binaries, and other artifacts. Preserve these; do not stage broad globs or use `git add -A`.

Important architecture materials also present in the workspace include `docs/architecture/target-agent-architecture.md`, `docs/architecture/target-architecture-v0.1.4.md`, `docs/roadmap/`, and `.scratch/`. They were not part of PR #11 and may be local-only.

## Suggested skills

Use the Skill tool for relevant skills before continuing:

- `code-review` to review PR #11 against the plan and repository standards.
- `superpowers:receiving-code-review` to evaluate new feedback against source evidence.
- `commenting-code` to maintain concise comments on lifecycle invariants and recovery behavior.
- `superpowers:verification-before-completion` to verify checks and PR state before reporting completion.
- `superpowers:using-superpowers` when starting fresh.

## Next steps

1. Check PR #11 and native Linux CI status when GitHub connectivity is available.
2. Read the committed plan and target architecture docs before extending implementation.
3. Preserve unrelated workspace changes and stage only requested files.
4. Reproduce and verify new review findings before changing code; update regression tests and explanatory comments with each fix.
