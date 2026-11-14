# PR-09 — Safe Infrastructure Diagnostics & Controlled Operations

**Status:** Phase 1 PASS; PR-09 Phase 2 IN PROGRESS. The user approved the narrow `container.restart` safety contract on 2026-11-15; implementation and final-head verification are in progress. The feature remains disabled unless the administrator sets the local opt-in and the target container carries its opt-in label. Current scope and evidence are tracked in the [PR-09 verification report](../../verification/v0.1.4a-next/pr-verification/pr-09-report.md). **Release target:** `v0.1.4a-next`.

## Current Phase 1 checkpoint

The read-only implementation is present on PR #32. It dispatches
`agent.diagnostics`, `container.inspect`, and `dependency.inspect` through
versioned request/result contracts, SQLite execution/audit/outbox state, and
authenticated NATS JetStream. The broker binding uses secret-free v1
provisioning metadata and a protected credential reference. Commit
`4f61193aab8a8456582d58b2a9faddc5e903d3b8` adds replacement/revocation
handling and real NATS session invalidation. Final PR head
`7ab6ee3b70307f49ec3d378e4c6d2e5575ef3cdf` passed Windows/Ubuntu CI and
native live Windows/Linux NATS verification.

Phase 2 is approved for implementation under [ADR 0004](../../adr/0004-opt-in-container-restart.md). Runtime registration remains off by default; both the local Agent opt-in and per-container label are required. Production NestJS Core interoperability is separately tracked and does not block Agent/Mock Core verification.

## Objective and exit boundary

Ship resource-targeted read-only Docker diagnostics plus optional opt-in container/service restart with durable authorization and idempotency; no SSH/arbitrary shell/AI remediation.

## Historical broader proposal (superseded by ADR 0004)

Execute PR-09 only after PR-07 and PR-08 pass. The operation names and endpoints
listed below are candidates from an earlier plan, not an approved operation
catalog or contract. Before implementation, establish the actual approved
request/response, authorization, target binding, idempotency, audit, timeout,
and result-reporting contracts. Implement only operations explicitly present
in those reviewed contracts. If no mutation policy is approved and enforceable,
keep all mutating operations disabled. This constraint supersedes any broader
scope implied by the candidate list in this plan.

The earlier candidate list below is not the approved PR-09 Phase 2 catalog. `container.logs`, service operations, discovery refresh, monitoring reconcile, a new Core HTTP endpoint, and any other mutation remain out of scope.

## Approved Phase 2 contract

- Core remains the business authorization authority; the authenticated scoped Core publisher is the source of commands. The Agent enforces local execution safety.
- Local opt-in: `AR_IMMS_OPERATIONS_CONTAINER_RESTART_ENABLED=true`; unset or any other value keeps the handler out of the registry.
- Per-target opt-in: `ar-imms.operations.container-restart.enabled=true` on a Docker container.
- Target: exact full 64-hex Docker ID in both target and parameters; inspect immediately before the action; require same ID, label, and `running` state.
- Side effect: one Docker Engine restart call, no retries. Docker uses the container-configured stop timeout or engine default.
- Verification: same ID with either an increased `RestartCount` or a strictly newer `StartedAt`; report container recovery separately. If verification is uncertain, persist `UNKNOWN` and never repeat the action on replay or startup recovery.
- Docker socket access may be host-level privilege; the label is an eligibility signal, not a security boundary.

## Baseline / current architecture constraints

Operations are not OpAMP `Restart` (that concerns the Agent). Core owns business authorization/audit; Agent enforces authenticated binding scope, expiry, exact target, local opt-ins, and outcome safety. Docker socket access is high privilege.

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
