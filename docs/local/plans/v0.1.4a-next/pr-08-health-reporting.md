# PR-08 — Self-Monitoring, Status & Diagnostic Evidence

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Provide truthful health reporting for Agent, Collector, managed dependencies, receivers and telemetry path, including offline and export failures.

## Baseline / current architecture constraints

Current `agenthealth` has readiness snapshots but full Core reporting/outbox and per-integration scrape evidence are not established. Avoid inventing delivery acknowledgment where Collector has no direct confirmation.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Define diagnostic model with separate phases: discovered, capability resolved, config accepted, generation applied, process ready, scrape observed, exporter healthy, backend delivery observed if independently measurable.
2. Collect self-telemetry from Agent and Collector using native health/metrics/logs where available; keep failures bounded and do not create monitoring dependency loops.
3. Integrate Windows Exporter, LHM Exporter and Linux dependency readiness/ownership diagnostics; distinguish unavailable hardware sensors from Agent failure.
4. Create status snapshot API/CLI and OpAMP health projection (only when supported by negotiated capabilities). Persist summary and diagnostic error codes, not unbounded log payload.
5. Provide bounded diagnostic bundle with redaction/TTL/size and timestamps; reconcile snapshot generation with real Collector process and gateway connectivity evidence.
6. Build deterministic fault-injection fixtures for Collector kill, exporter kill, gateway outage, invalid config, Docker shutdown, Core offline and disk exhaustion.

## Edge cases and failure semantics

- Collector process ready but Redis scrape fails: do not report healthy monitoring.
- Gateway 503/blackhole: distinguish exporter retries from proven delivery; no zero-loss guarantee.
- Agent cannot reach Core: local health remains available; retry bounded with backoff.
- LHM not supported by hardware: show UNSUPPORTED capability, not failure of baseline.
- Full disk/log overflow must not wedge Agent or leak credentials.

## Acceptance / verification gates

- Agent local status and Core status show OS/Agent/Collector/exporter/config states separately.
- Fault injection and recovery evidence on Windows/Linux, with transitions timestamped.
- Diagnostic bundle redacts secrets and is bounded in memory/disk/output.
- OpAMP reporting must not block monitoring or Collector lifecycle.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Revert new reporting projections without changing Collector desired/active generation; retain local status baseline.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
