# PR-07 — OpAMP Supervisor Adoption & Control Plane Integration

**Status:** IN_PROGRESS — PR-07a Windows and Ubuntu WSL live Supervisor/Collector and profile-parity tests pass with Docker Desktop caveat; PR-07b Go Mock Core and pinned Supervisor auth pass on Windows and WSL. Native Linux Docker Engine verification, Agent-owned process-tree recovery/credential provisioning, and controlled lifecycle transfer remain open.
**Release target:** `v0.1.4a-next`. **Baseline:** PR-06 merge
`af2f5db4c6b8832bc7bcc87d332d899dad01b452`; check current `HEAD` before each
slice. See [target contracts and ownership](../../architecture/v0.1.4a-next/opamp-integration.md), [verification report](../../verification/v0.1.4a-next/pr-verification/pr-07-report.md), and [release progress](release-progress.md).

## Objective

Adopt the official OTel OpAMP Supervisor only after exact-version Windows/Linux
capability and parity checks, and deliver a narrowly scoped remote-management
vertical slice using a Go OpAMP server and contract-compatible Go Mock Core.
Transfer Collector lifecycle ownership once, only after parity. Core production
changes and shared-contract publication require separate approval.

The existing Agent `internal/supervisor` is not the OTel OpAMP Supervisor.
Preserve current Agent Runtime, enrollment, SQLite, CLI, dependencies, LKG,
PR-05 discovery, PR-06 profiles, and OTLP behavior through migration.

## Work slices and commit boundaries

The a/b/c slices are gated checkpoints, not exactly three GitHub PRs. Keep
commits small and scoped, for example: artifact/provenance evidence; a single
capability finding; contract specification; Mock Core API; OpAMP adapter; Agent
provisioning; controlled lifecycle transfer. Each commit should be independently
reviewable and avoid unrelated formatting or worktree changes.

### PR-07a — Capability spike (no runtime ownership transfer)

1. Preserve the PR-06 merge baseline and current Agent Runtime. Keep the
   capability harness opt-in and test-only; no lifecycle ownership transfer.
2. Verify exact Windows/Linux amd64 Supervisor artifacts, version, SHA-256,
   provenance, license, and compatibility with pinned Collector Contrib
   v0.160.0. Do not infer compatibility from matching version strings.
3. Verify OpAMP extension and config support, existing host/Redis/PostgreSQL/
   Prometheus profiles, local config updates and deterministic composition.
4. Test valid and invalid remote config, startup with no server, offline
   fallback, Supervisor and Collector crash/restart, rollback, recovery, and
   actual OTLP delivery. Record whether config changes restart Collector and
   whether existing LKG guarantees cover activation failures.
5. Run separately on native Windows and native Linux. Docker Desktop evidence
   is not native Linux evidence. Retain exact commands, environment, logs,
   artifact hashes, and sink telemetry.
6. Deliver migration and rollback design. Keep the existing Agent Runtime the
   sole Collector lifecycle owner until PR-07c.

### PR-07b — Contracts and control-plane vertical slice

Before dependent implementation, inspect the authoritative `agent-enrollment`
OpenAPI and Core API Baseline. The authoritative files have been read at
`agent-enrollment.openapi.yaml` blob `5ce0f6a3d94401c2adf65eaf9ad524a37b868571`
and Core `api/baseline.yaml` blob `4f4166f2a4d1770f5142618d58b1997092d289c0`.
The enrollment spec defines single-use bootstrap semantics and recovery, not an
Agent bearer-authenticated control route. Core reserves `agentBearer`, but the
baseline defines no Agent control API. Reuse those conventions; do not infer or
implement unsupported production Core routes, nor publish shared contracts.

Define and review explicit contracts for installation authentication and
binding, Supervisor credential provisioning/rotation/revocation, OpAMP
`instance_uid` mapping, targeted versioned desired policy, Gateway endpoint
validation, local/remote config ownership and merge/deletion rules, revision and
hash semantics, status/error mapping, idempotency, retry, cache and reconnect.
Keep desired revision distinct from applied revision; state what evidence each
status represents.

Proposed first policy field: Core-owned OTLP Gateway endpoint. Agent continues
to own installation/infrastructure identity, monitoring profiles, receiver
rules, and protected local configuration. This proposal is not authorization
for broader remote policy.

Implement a small Go OpAMP server using `opamp-go` and a Go Mock Core that
implements the same documented HTTP contracts intended for Core. Test-only
policy mutation and failure injection must not leak into production interfaces.
Do not couple server code to mock internals or create a second production
business database. Mock interoperability is not production NestJS Core E2E.

### PR-07c — Runtime adoption and E2E

Only after PR-07a parity and PR-07b contract gates pass: provision the pinned
Supervisor as a managed dependency, transfer Collector process ownership once,
and remove/disable the former owner only as part of that verified transition.
Prove rollback restores a single prior owner. Verify native install/upgrade,
service lifecycle, local config changes, offline fallback, reconnect/restart,
and PR-05/06 receiver/profile parity with real telemetry on Windows and Linux.

## Mandatory verification gates

- Exact checksummed artifacts and pinned-version compatibility on each OS.
- No overlapping Agent/OTel Supervisor ownership of one Collector.
- Target scoping, authenticated identity binding, credential rejection and
  revocation behavior; no trust in self-reported `instance_uid`.
- Local config and protected Supervisor settings cannot be overwritten outside
  approved policy fields; invalid/stale/conflicting policy fails safely.
- Configuration hash/revision status remains truthful through restart,
  rollback, server outage, duplicate request, and older applied config.
- Offline startup/recovery, Supervisor/Collector crash, candidate activation
  failure, and actual OTLP sink delivery are separately evidenced.
- Existing enrollment, SQLite, CLI, managed dependencies, discovery, monitoring
  profiles, and LKG behavior remain compatible.
- Native Windows and native Linux E2E; report mock-backed and production Core
  evidence separately. Untested gates remain `NOT RUN` or `BLOCKED`.

## Not in scope / stop conditions

No arbitrary commands, remote package updates, remote container operations,
unrestricted Collector YAML, full Agent Registry CRUD, second policy database,
or silent modification of the Core repository or `docs/shared` submodule. Stop
before implementing a contract whose authoritative Core conventions are
missing; record the concrete gap and continue independent capability work.

## Verification log and merge audit

Before merge, `pr-07-report.md` must list each slice's exact commit/PR head,
platform, OS build, Agent/Supervisor/Collector versions and hashes, Docker and
sink versions, commands, raw log/evidence locations, PASS/FAIL/BLOCKED/NOT
RUN/UNSUPPORTED, deviations, known failures, owner transfer state, and rollback
result. Audit that the report describes the exact candidate head and that CI
and required native E2E ran after the final documentation/test-log update.
Never merge with a mandatory gate unresolved; keep the checkpoint draft while
verification is incomplete.
