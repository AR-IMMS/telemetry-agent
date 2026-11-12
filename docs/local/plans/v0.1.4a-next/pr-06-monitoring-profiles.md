# PR-06 — Application Monitoring Profiles & Extensible Provisioning

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Ship Docker application monitoring for Redis, PostgreSQL and Python `/metrics`, plus a config-only extension path and a mock facilities integration without reinventing OTel receivers.

## Baseline / current architecture constraints

Built-in Windows Exporter/LHM/LHM Exporter, Linux hostmetrics/optional Node Exporter, Redis Integration and pinned receiver capabilities must remain functional. Profiles are AR-IMMS schema, NOT raw receiver_creator config.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Write schema/versioning/validation for a compact MonitoringProfile (`identity`, `match`, `capability`, `endpoint`, `settings`, optional secret refs); reject arbitrary Collector snippets and ambiguous target matches.
2. Implement capability catalog from built-in Collector manifest: native Redis/PostgreSQL/Prometheus first, with precise cross-platform unsupported status.
3. Implement profile resolver/provisioner: discovery metadata -> deterministic compatible receiver-creator rules or authorized static OTel configuration, depending on actual capability and ownership.
4. Preserve host baseline: OTel hostmetrics plus managed Windows Exporter and optional LHM/LHM Exporter; Linux optional Node Exporter; do not double-count host metrics without documented selection.
5. Create real Docker Compose fixtures (pin digests at test time) with redis:alpine, postgres:alpine, Python:alpine Prometheus server; real Postgres database and restricted scraping credentials.
6. Demonstrate a new Python app with a different name/port and a mock cooling/power exporter added using configuration/profile only, without rebuilding Agent/Collector.
7. Document DX validation/explain command or API: discovery evidence, profile match, capability, receiver activation/readiness, scrape/export evidence and actionable failures.

## Edge cases and failure semantics

- No credentials: report `requires_configuration`, avoid accidental anonymous auth and secret leakage.
- Missing Collector receiver: reject profile before application; external exporter/OCB is an explicit future path.
- Unsupported/unknown service: leave safe unmonitored state; no approval workflow required.
- Regex/labels from Docker are untrusted; bound matching and config size, avoid config injection.
- Docker Desktop endpoint, multiple Compose projects, same port in separate networks, duplicate receiver rule, IPv6.

## Acceptance / verification gates

- Linux and Windows actual OTLP sink sees Redis, PostgreSQL and Python app metrics with correct resource identity.
- New profile and mock cooling/power `/metrics` require no Go source or binary rebuild.
- Start/recreate/remove/scale converges with no ghost or duplicate receiver.
- Installed Win Exporter/LHM dependency ownership, Linux hostmetrics and last-known-good regression pass.
- README shows smallest working profile and exact configuration/diagnostics path.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Keep last active profile revision and LKG; revert new rule generator/profile adapters while keeping old Redis path. Invalid profile must not overwrite active config.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
