# v0.1.4a-next — implementation plan index

**Status:** BLOCKED — PR-01 local implementation is complete; integration review and publication await environment gates. Later checkpoints remain conditional on their reviewed contracts and verification gates. Reconcile each plan's source paths and evidence
against current HEAD before its work begins. See the [documentation integration
status](../../architecture/v0.1.4a-next/integration-status.md).

**Release goal:** installable and verified Linux/Windows `v0.1.4a-next`, not just a source refactor. All plans are authored now; they are **review-ready conditional plans**, not claims that code is implemented or tests passed. Before each PR starts, reconcile source paths and baselines against current HEAD and previous PR verification.

| PR | Plan | Dependencies / must-prove | Scope |
|---|---|---|---|
| 01 | [Foundation](pr-01-foundation.md) | Current baseline snapshot | Docs, coding guardrails |
| 02 | [SQLite & events](pr-02-events-sqlite.md) | 01 | Technical persistence + advisory dispatcher |
| 03 | [App composition](pr-03-app-composition.md) | 02 | CLI split, service host |
| 04 | [Context migration](pr-04-context-migration.md) | 03 | Repositories and ownership; split 04a–c if needed |
| 05 | [OTel discovery](pr-05-otel-discovery.md) | 04, pinned manifest parity | Observer/Receiver Creator spike and gated transfer |
| 06 | [Monitoring profiles](pr-06-monitoring-profiles.md) | 05 and capability inventory | Redis/PostgreSQL/Python + mock facilities |
| 07 | [OpAMP & Supervisor](pr-07-opamp.md) | 04, PR-05/06 parity, exact OTel capability spike, reviewed contracts | 07a capability, 07b contracts/mock vertical slice, 07c controlled runtime adoption |
| 08 | [Health & reporting](pr-08-health-reporting.md) | 07 where OpAMP reporting required | Honest health and diagnostic evidence |
| 09 | [Controlled operations](pr-09-controlled-operations.md) | 03, approved security ADR | Diagnostics + opt-in mutation |
| 10 | [Release hardening](pr-10-release-hardening.md) | 01–09 gates | Real-host install, upgrade, E2E and release verification |

Plans 05/07/09 have explicit **go/no-go decisions**. Their blocked gates cannot be silently deferred while claiming full release. Context migration can be split into smaller PRs without changing contract. PR 07 and PR 09 may be developed in parallel *after* their prerequisites but remain independently gated.

PR-07's immediate target is a Go OpAMP server plus contract-compatible Go Mock
Core; production NestJS Core interoperability and shared-contract publication
remain separate approval gates. The three PR-07 slices are checkpoints, not a
requirement for exactly three GitHub PRs. The ≤450-line automated gate in these plans supplements the shared policy: files
above its usual 300–400-line range still need justification.

## Deliverables per PR

1. `pr-XX-*.md`: baseline, touched files, implementation, negative tests and rollback.
2. Code + tests; architectural/coding policy CI gates.
3. `docs/local/verification/v0.1.4a-next/pr-verification/pr-XX-report.md` with Git SHA, platforms, commands and **PASS/FAIL/BLOCKED/NOT RUN/UNSUPPORTED** evidence.

Never treat this plan, current upstream documentation, or historical Windows
Redis evidence as proof of pinned Supervisor artifact compatibility or E2E
success. Do not transfer Collector lifecycle ownership until exact-version
Windows/Linux parity is proven. Any production Core implementation belongs in
`core-backend` and needs separate approval and contract review.

## Live release progress

Track the current execution state and evidence in [release progress](release-progress.md). The checkpoint report directory is [docs/local/verification/v0.1.4a-next/pr-verification/](../../verification/v0.1.4a-next/pr-verification/).
