# PR-05 — OTel-Native Discovery Spike & Gated Receiver Ownership

**Status:** IN_PROGRESS. **Release target:** `v0.1.4a-next`. The spike is accepted and implementation is authorized against the [approved discovery and monitoring contracts](../../architecture/v0.1.4a-next/discovery-monitoring-contracts.md). The checkpoint must not be marked PASS until Windows and native Linux parity gates pass.

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

**Evidence to attach:** see the current [PR-05 verification report](../../verification/v0.1.4a-next/pr-verification/pr-05-report.md) for before/after state, commands, platform and Docker/Collector versions, evidence levels, Git SHA, and items NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Switch receiver ownership back to legacy Agent materialization using one-way feature flag; rollback cannot leave both active. Preserve old working templates and config. If component unavailable, revert spike-only runtime path.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.

## Historical spike ruling before Agent implementation (2026-11-12)

The following records the state at the end of the capability spike. Its
verification gaps were subsequently addressed in part by the implementation
evidence below; retain it as historical context, not as the current status.

- The exact pinned v0.160.0 Linux and Windows binaries contain `docker_observer`, `receiver_creator`, and `redis`; Linux WSL2 and Windows Docker Desktop live probes produced Redis metrics. On Linux WSL2, the container bridge endpoint was unreachable from the host-side Collector. An Observer `alternate_port` configuration worked for single- and two-container probes, including removal and recreation. These are capability findings, not a production ownership transfer.
- The user approved Core `node_id`, Docker container `resource_id`, optional label `workload_id`, and container-name display-only semantics. Stable `workload_id` is no longer a blocker.
- Implementation ruling: use the existing Docker discovery opt-in endpoint, enable the native profile only when the Core binding is available, keep profile rules independent of per-container endpoints, and route only label-matched Docker Redis resources to Receiver Creator. Existing Agent receiver contributions remain the legacy fallback for unlabelled or manual targets.
- Endpoint strategy: Windows uses `alternate_port` through loopback; Linux prefers the published mapped port when available and otherwise uses the observed container endpoint. The observed endpoint is never treated as proof of reachability.
- The runtime reconciliation path can activate a new generation when the desired set changes even if rendered bytes do not. PR-05 must therefore avoid creating/deleting per-container Agent desired instances for native-owned targets; Agent inventory continues to track those resources, while Collector rules remain stable across churn.
- The acceptance matrix at spike completion remained incomplete: previous probes used the debug exporter, and Windows churn, native Linux host, Docker access failure/recovery, and some endpoint cases remained to be run.

## Implementation evidence and current ruling (2026-11-12)

- The native Docker/Redis profile is now composed into the Agent when its explicit Docker endpoint, registered Core Node ID, and pinned Redis capability are available. Agent inventory remains active, while Agent-generated Redis contributions are suppressed only for opted-in native-owned targets.
- The latest Windows/amd64 live test used real Docker Redis containers, the pinned v0.160.0 Collector, the Agent runtime, and an isolated OTLP sink. It verified private port 6380, Receiver Creator uniqueness, duplicate-point absence, identity attributes, and unchanged rendered config/generation across churn.
- Windows full Go tests, vet, build, exact profile validation, architecture guard, SQLite polling stress, and Linux/amd64 cross-build pass. The cross-build is not native Linux verification. Pinned Collector candidate validation and activation failures restore the previous LKG state.
- PR-05 remains **IN_PROGRESS / CONDITIONAL**. Updated native Linux Docker daemon outage/recovery, post-recovery delivery, and live endpoint failure diagnostics remain open. See the [PR-05 verification report](../../verification/v0.1.4a-next/pr-verification/pr-05-report.md) for commands, versions, evidence levels, and untested cases.

## Implementation plan

### Final closure execution plan (2026-11-12)

The user accepted Windows/Linux live OTLP delivery and LKG verification. PR-05 remains IN_PROGRESS until the closure gates below pass. Do not create another receiver lifecycle engine; use the pinned Observer/Receiver Creator and the Agent's existing lifecycle, local diagnostics, and inventory stores.

1. Verify exact v0.160.0 Docker Observer multi-network endpoint selection and Receiver Creator lifecycle IDs. Pin deterministic observer configuration, retain raw Collector lifecycle logs for the E2E, and assert at most one active receiver per Collector instance/resource/profile plus no duplicate OTLP metric points.
2. Add the optional `ar-imms.monitoring.port` profile override contract, defaulting Redis to 6379. Validate decimal ports in `1..65535`, fail closed on invalid values, and keep generic discovery unaware of profile semantics.
3. Route parsed Collector scrape failures into the existing local Agent diagnostics surface. Expose missing, invalid, ambiguous, resolved/unverified, and unreachable states with observation evidence and timestamp; do not turn runtime errors into inventory deletion or desired-state changes.
4. Add isolated Docker-in-Docker recovery E2E on native Linux. Stop and restart only the isolated daemon, prove inventory preservation during outage, and verify receiver/telemetry convergence after recovery without an Agent-driven Collector restart when the monitoring policy is unchanged.
5. Reproduce the Windows SQLite polling failure on normal host access and under repeated/stress runs. Retain first-failure and retry evidence; fix only if source and repeated evidence establish a product timing defect.
6. Run focused tests, pinned Windows/Linux Collector validation, Windows and native Linux Docker/Collector/OTLP E2E, full CI, update this plan plus the contract/report/progress tracker, review the complete diff, then close/merge PR-05 only if every mandatory gate is PASS.

| Closure gate | Status | Evidence |
|---|---|---|
| Receiver uniqueness and duplicate telemetry | VERIFYING | Final Windows live E2E asserts one active receiver per Collector instance/endpoint and no duplicate OTLP points; native Linux rerun remains required. |
| Isolated Docker daemon outage/recovery | VERIFYING | Linux DinD CI harness stops/restarts only its isolated engine and checks inventory, receiver/telemetry convergence, config generation, and Collector instance identity; updated workflow is pending CI. |
| Live endpoint diagnostics | VERIFYING | Sanitized Collector scrape evidence with UTC observation time feeds the status API and `inventory inspect`; Linux E2E pauses Redis and requires `endpoint_unreachable`; updated workflow is pending CI. |
| Validated per-profile target port | VERIFYING | Redis defaults to 6379 and supports `ar-imms.monitoring.port`; invalid values fail closed and Windows E2E delivers from private port 6380. Native Linux parity remains required. |
| Windows SQLite polling investigation | PASS (flaky test classified) | Run 43239629059 failed once at the test's one-second startup-reconciliation wait; its CI retry and host-access `-count=100` rerun passed. The failure is consistent with runner scheduling sensitivity; no product regression reproduced and no code change was justified. |

### Task 1: Profile contract and generated Collector rule

**Files:** create `internal/monitoringprofile/doc.go`, `profile.go`, `docker_config.go`, and focused tests. Add an explicit OTel configuration contract for `docker_observer` and `receiver_creator` with platform endpoint strategies, label rules, `node_id`, `resource_id`, optional `workload_id`, display name, Redis capability, and a stable config independent of container inventory.

**Tests:** profile registry rejects duplicate/invalid profiles; Windows uses only `alternate_port`; Linux prefers `alternate_port` and falls back to the observed endpoint; labels require both enabled and `redis`; rendered config carries each identity attribute and wires `receiver_creator` into metrics.

- [x] Write profile and Collector-layer tests first; confirm they fail because the profile API is absent.
- [x] Implement the smallest built-in profile registry and Docker Collector config projection.
- [x] Run `go test ./internal/monitoringprofile` and confirm it passes.

### Task 2: Inventory labels and Agent ownership suppression — implemented

**Files:** extend `internal/discovery/docker/provider.go` allowlist and provider tests; add a profile ownership projection used by `cmd/agentctl/template_materialization_runtime.go` and `cmd/agentctl/reconciliation_runtime.go`; preserve all other template and manual Integration behavior.

**Tests:** allowlist only the three approved AR-IMMS labels; profile-matched Docker resources are excluded from Agent-generated Redis receiver materialization; bound manual Redis Integrations matching those resources resolve with no receiver contribution; unlabelled and unrelated targets retain the legacy Redis contribution; provider errors retain inventory.

- [x] Add coverage for the approved label allowlist and profile-specific ownership projection.
- [x] Implement label normalization, native-target template filtering, and fail-closed ownership suppression.
- [x] Run focused discovery, ownership, and materializer tests.

### Task 3: Runtime composition and existing LKG handoff — implemented; profile failure paths verified on Windows

**Files:** compose the profile layer in `cmd/agentctl` bootstrap and reconciliation baseline paths; include `receiver_creator` as a metrics receiver only when Docker endpoint and Core `node_id` are available. Preserve current candidate validation, activation journal, readiness, and LKG recovery; add no SQLite migration.

**Tests:** registered Node ID enables native rule composition; missing/failed enrollment or unset Docker endpoint leaves legacy baseline unchanged; changing container inventory does not change the profile layer or Collector candidate; a failed candidate restores the prior active configuration.

- [x] Add runtime baseline composition and fingerprint tests.
- [x] Compose native rules through bootstrap and reconciliation; require explicit Docker endpoint, registered Core Node ID, and pinned Redis capability.
- [x] Keep the existing SQLite schema, candidate validation, activation, readiness, and LKG paths. Pinned v0.160.0 profile-composed validation rejection and Docker Observer activation/readiness failure both restore prior configuration/state on Windows.

### Task 4: Live parity, evidence, and release progress

**Files:** add opt-in Agent-level Docker/Redis E2E coverage, endpoint inventory diagnostics, recovery tests, architecture contract, and verification evidence.

**Verification:** exact v0.160.0 component validation; full Windows Go tests/vet/build and Windows live Docker/Collector/OTLP test; native Linux Go tests/vet/build and Linux live Docker/Collector/OTLP test where available; Docker recovery, endpoint failures, add/remove/recreate, multi-instance identity, no per-container Collector generation, and LKG rollback. Mark each result with environment and evidence state: discovered, receiver active, collection verified, delivery verified.

- [x] Run the Windows Go suite, vet, build, Linux/amd64 cross-build, exact v0.160.0 config validation, and isolated Windows Docker + OTLP sink E2E.
- [x] Extend Windows live E2E to a nondefault published host port, a private port 6380 override, a multi-network unpublished target, multi-instance identity, recreation, uniqueness, duplicate-point checking, and OTLP delivery.
- [x] Add provider recovery unit coverage, missing/ambiguous/resolved inventory diagnosis, pinned Receiver Creator error parsing, profile registry extensibility coverage, and retain generic candidate validation/LKG suite evidence.
- [ ] Verify actual isolated Docker daemon outage/recovery with Agent inventory, successful discovery after recovery, OTLP receiver convergence, stable Collector instance, and unchanged config generation. The Linux DinD test is implemented but requires fresh CI evidence.
- [ ] Feed matching OTel scrape-error evidence into Agent status and `inventory inspect`, then verify live `endpoint_unreachable` against a paused Redis fixture on native Linux.
- [x] Verify receiver uniqueness and duplicate-point absence in the latest Windows live E2E; native Linux rerun still required. The multi-network diagnostic selection uses Docker `GwPriority`, then network name.
- [x] Replace the private-port-only constraint with a validated Redis default/override contract; Windows private-port 6380 delivery passed. Run the same pinned test on native Linux.
- [x] Induce profile-composed candidate validation and Docker Observer activation/readiness failures with the pinned Collector; verify LKG restoration and no native receiver configuration remains after rollback.
- [x] Run native Linux CI suite/vet/build and live Docker + pinned Collector + OTLP sink E2E on GitHub `ubuntu-latest`; run 43238401049 passed. Local Windows cross-build remains distinct from this native Linux evidence.
- [x] Record Windows and Linux evidence states, limitations, and progress in the verification report, [Windows live evidence](../../verification/v0.1.4a-next/pr-verification/pr-05-windows-live-evidence.md), and [Linux live evidence](../../verification/v0.1.4a-next/pr-verification/pr-05-linux-live-evidence.md).
