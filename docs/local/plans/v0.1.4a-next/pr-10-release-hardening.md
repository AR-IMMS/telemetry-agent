# PR-10 — Packaging, Cross-Platform E2E & Release Hardening

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Turn feature-complete source into an installable `v0.1.4a-next` artifact with real acceptance evidence for native Linux and Windows, upgrade/recovery and dependable uninstall.

## Baseline / current architecture constraints

Prior Windows Redis telemetry proof is historical, Linux native Docker proof was blocked in previous environment; every version-dependent claim must be re-run for the candidate binary and exact build.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Freeze release candidate commit, generate SBOM/dependency inventory and pinned checksums for Agent, Collector, and managed exporters; document compatibility matrix and license notices.
2. Produce native Linux amd64 package/service (`systemd`) and Windows amd64 service/installer (`SCM`), preserve upgrade state and secure credential storage. Validate environment-specific native tests, not just cross-compile.
3. Build repeatable Linux/Windows Docker Compose Redis/Postgres/Python Alpine-oriented fixtures; fix image digests and exact Collector component manifest. Separate Agent host from Gateway/test sink.
4. Run complete release matrix A1–A12: fresh install, enroll, reboot, service crash, dependency ownership, Docker lifecycle, profiles, OpAMP remote config, self-health, operations, security and upgrades.
5. Run no-op and bad-config recovery tests; kill Collector during activation; simulate Core/Gateway outages, partially failed Docker snapshots, concurrent updates and Agent restart.
6. Perform clean uninstall and reinstall: remove only Agent-owned services/exporters; preserve or securely remove secrets according to documented flags; do not delete third-party dependencies.
7. Create operator/developer docs, upgrade/rollback/troubleshooting runbooks; publish signed/checksummed artifacts and release verification report with exact PASS/FAIL/BLOCKED/UNSUPPORTED.
8. Run smoke test from pristine OS to Core enrollment and actual OTLP metrics, not only unit test runner.

## Edge cases and failure semantics

- Windows Docker Desktop networking vs native Linux Docker routing: prove actual scrape address and container namespace.
- Unsupported hardware must be UNSUPPORTED with reason, not PASS; LHM optional capability may not exist.
- Collector binary missing required receiver/observer: release gate FAIL or approved dependency distribution change.
- Install/upgrade interrupted halfway: no corruption, resume or rollback; no duplicate services.
- Old DB to new DB and rollback path remains safe; credential secret loss is release blocker.
- Feature flags cannot silently conceal missing mandatory requirements.

## Acceptance / verification gates

- A1–A12 each have platform-specific actual evidence, commit, digest, logs/metrics, tester and result.
- Fresh native Linux and Windows install/enroll/monitor/remote-update/recovery/operation E2E pass.
- Package artifacts checksum verified; upgrade/reinstall/uninstall respect dependency ownership.
- No unresolved high-severity security failure, migration regression, duplicate receiver owner or OpAMP cross-node misrouting.
- If gates unmet, do not tag complete `v0.1.4a-next`; publish blockers and scope decision instead.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Roll back to previous release binaries/config using documented compatibility path; preserve SQLite/credentials, reverse only compatible additive migrations, retain LKG and managed dependencies.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
