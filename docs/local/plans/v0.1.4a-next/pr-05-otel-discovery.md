# PR-05 — OTel-Native Discovery Spike & Gated Receiver Ownership

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Prove whether pinned Collector Docker Observer + Receiver Creator can own dynamic receiver lifecycle for Redis and other supported workloads; delegate only after parity. Keep Agent discovery as extensible metadata/inventory support, not a second telemetry execution engine.

## Baseline / current architecture constraints

Collector pinned v0.160.0; Docker inventory, Template materialization, deterministic compiler and Redis E2E already exist. Upstream README does NOT prove bundled binary compatibility on either platform.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Inventory actual bundled Collector components and validators on native Linux/Windows. If `docker_observer` or `receiver_creator` is missing, stop rollout; file explicit OCB distribution/build decision (separate approved change).
2. Write isolated POC Collector config using Docker Observer + Receiver Creator + native Redis receiver; compare against existing Redis telemetry and resource attributes.
3. Exercise add/remove/recreate/scale, nondefault ports, multiple networks, unexposed container ports, Docker socket permissions and Windows Docker Desktop VM/network visibility.
4. Prototype a minimal provider contract `Snapshot(ctx)` with source/scope and complete-vs-failed snapshot provenance; normalize metadata from Docker; document optional future OS process/network and cooling/power provider seams.
5. Introduce stable capability-to-rule templates only for installed receiver types; do not auto-create unsupported types. Runtime config compiler verifies rule templates against actual manifest.
6. If parity passes, switch one target type behind a single-owner migration flag (Agent materializer OFF for delegated receivers). Retain durable inventory/history for Core correlation; no other target changes.
7. Ensure container instance churn does not regenerate Collector config or trigger needless restarts; new capability rule/policy changes may cause activation.

## Edge cases and failure semantics

- Docker API and Observer disagree on endpoints: mark source-specific evidence, do not guess reachability.
- Unmatched service is discovered but not falsely `monitored`; no proposal/approval UI required for supported policies.
- Duplicate endpoint rules, dual owners and wrong resource attributes are release blockers.
- Docker daemon disconnected, partial event history, container IP changes; convergence after resync.
- Windows host vs Docker Desktop Linux VM routing may make receiver endpoint unreachable.

## Acceptance / verification gates

- Linux and Windows real Docker + pinned Collector E2E: Redis metrics reach isolated OTLP sink.
- Multi-instance metrics correctly attributed; no ghosts after defined convergence and no duplicate scrape.
- No needless Collector generation on container recreate under same rule.
- Old Agent discovery history/identity/CAS retained; full rollback to legacy materializer demonstrated.
- Decision record marks PASSED and delegated scope, or FAILED/BLOCKED plus explicit release gap; a failed spike is not a completed OTel-native feature.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Switch receiver ownership back to legacy Agent materialization using one-way feature flag; rollback cannot leave both active. Preserve old working templates and config. If component unavailable, revert spike-only runtime path.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
