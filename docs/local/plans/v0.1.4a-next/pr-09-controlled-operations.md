# PR-09 — Safe Infrastructure Diagnostics & Controlled Operations

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Ship resource-targeted read-only Docker diagnostics plus optional opt-in container/service restart with durable authorization and idempotency; no SSH/arbitrary shell/AI remediation.

## Baseline / current architecture constraints

Operations are NOT OpAMP `Restart` (that concerns Agent). Core owns human RBAC/approval/audit; Agent must independently enforce narrowed signed/secure operation intent and exact target verification. Docker socket access is high privilege.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Write explicit security ADR before mutation: executor identity, transport/auth, scope/expiry, per-target capability, audit, storage, local privilege boundaries and allowed OS service list.
2. Implement structured `container.inspect`, bounded `container.logs`, `service.status`, `discovery.refresh`, `monitoring.reconcile` via context-owned use cases; validate request schema and responses.
3. Define durable operation record in SQLite: operation_id, target native immutable ID, requested actor/intent, start/deadline, outcome, audit ref, idempotency conflict state. Any schema change requires explicit versioned migration.
4. Implement Core endpoint (separate from OpAMP) with IAM and auditable request dispatch. Agent validates instance-bound authorization, target freshness, expiry, operation allowlist and concurrency before execution.
5. If security ADR approved, add `container.restart` and allowlisted `service.restart` adapters on Linux/systemd and Windows/SCM; avoid raw shell commands; no privilege escalation without documented installation grant.
6. Post-operation verification reads observed container state and telemetry evidence; report action outcome separately from incident recovery.
7. Provide operator UX/CLI docs and E2E tests including retry, duplicate, stale target, Docker daemon unavailable, lost result, cancellation and Agent reboot.

## Edge cases and failure semantics

- Same operation ID delivered twice: never execute twice, including crash/reboot and duplicate transport delivery.
- Container was recreated between authorization and execution: reject stale target; use immutable container ID.
- Timeout/connection loss after starting mutation: UNKNOWN until proven; never auto-retry restart.
- Denied/out-of-scope request: reject before Docker/OS calls; emit audit.
- Container restart succeeded but app remains unhealthy: mark operation success, recovery failed.
- Host privilege may be effectively root via Docker socket: must be documented/mitigated; P1 mutation disabled if not enforceable.

## Acceptance / verification gates

- Native Linux and Windows read-only diagnostics on real Docker containers, bounded logs and redaction.
- Core IAM + Agent authorization/expiry/replay/target/idempotency tests pass.
- If enabled: exactly one restart, correct container, durable outcome across Agent restart, precise auditing.
- No arbitrary command endpoint, SSH, or OpAMP piggyback; OS service allowlist enforced.
- If mutation security cannot be demonstrated, ship disabled and mark release A10 mutation gate FAIL rather than claiming complete full-scope release.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Disable mutating ops feature at Core and Agent; preserve inspect and reporting; DB migration additive with downgrade plan if new tables introduced.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
