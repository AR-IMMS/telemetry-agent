# PR-01 — Architecture foundation and release baselines

**Status:** BLOCKED — local implementation and verification are complete; integration review/publication await environment gates. The earlier blanket “do not implement” gate is superseded by the 2026-11-12 authorization; target boundaries remain flexible and contracts are due before dependent work. See the [PR-01 checkpoint report](../../verification/v0.1.4a-next/pr-verification/pr-01-report.md).

## Objective

Introduce/review repo documentation, dependency guardrails, verified regression baseline, feature evidence inventory and enforceable coding policy **without runtime behavior changes**.

## Current behavior and evidence

The Phase-1 audit at `477e831d` found `cmd/agentctl` is an application host, `agentstate` a cross-domain SQLite hub, and LKG activation/reconciliation already working. Working-tree cleanliness was not established; before any code change check current HEAD, index, submodule status, real CLI and test baseline. This plan does not claim present CI/E2E success.

## Implementation steps

1. Compare target docs against current production imports and update exact ownership matrix where source differs.
2. Add context-level `doc.go` for existing packages **without mechanical package moves**; document real invariants/security.
3. Introduce import-direction checks with explicit transitional allowlist; add production Go ≤450 LOC check and formatter/linter commands.
4. Capture CLI help/flags/output snapshots, sqlite old-state fixtures, Collector binary capabilities, pinned manifest/checksum and Windows/Linux baseline test commands.
5. Record existing Redis/Template E2E evidence as **historical**; identify what must be re-run.
6. Add CI validation and prepare PR verification report.

## Non-goals

No OpAMP protocol implementation, Event Dispatcher implementation, new discovery providers, receiver_creator migration, new schema migrations, CLI changes, or operations executor.

## Invariants

No modified runtime configuration generation, no SQLite table changes, no collector artifact upgrade, no altering installed dependency ownership, no accidental breaking of shared docs submodule.

## Edge cases

`.git/modules/docs/shared` access denied; CI OS variance; audit commit differs from HEAD; existing local changes; stale tests or missing Docker/Windows tooling. Report BLOCKED and stop affected validation rather than fabricating PASS.

## Verification & rollback

Run gofmt/`go test ./...`/`go vet ./...`, cross-compile where available, architecture allowlist and size gate; compare CLI/fixture snapshots. Rollback removes docs/checks only; no data migration. **Exit criterion:** owner-reviewed baseline with traceable evidence and non-breaking CI guardrails.
