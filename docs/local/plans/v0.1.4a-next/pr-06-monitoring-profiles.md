# PR-06 — Application Monitoring Profiles & Extensible Provisioning

**Status:** IN_PROGRESS — implementation on `feat/pr-06-monitoring-profiles`. **Release target:** `v0.1.4a-next`. PR-05 merged at `11fdd1e720c542898ba9c0c673063b1d698d7451`.

## Objective and exit boundary

Ship Docker application monitoring for Redis, PostgreSQL and Python `/metrics`, plus a config-only extension path and a mock facilities integration without reinventing OTel receivers.

## Baseline / current architecture constraints

Built-in Windows Exporter/LHM/LHM Exporter, Linux hostmetrics/optional Node Exporter, Redis Integration and pinned receiver capabilities must remain functional. Profiles are AR-IMMS schema, NOT raw receiver_creator config.

## Verified implementation direction

- Local data-only profiles are read from `<config-root>/monitoring-profiles/`; versioned typed fields are parsed with unknown-field rejection and file/profile limits. The `examples/` child directory is not loaded automatically.
- Existing Redis remains the built-in default. Custom profiles use only the pinned Redis, PostgreSQL, and Prometheus native receiver mappings; Docker discovery and the Observer/Receiver Creator lifecycle are reused.
- PostgreSQL credential fields are environment-variable references only. No values enter profile files, state, or logs. Core/OpAMP authority and remote secret delivery remain out of scope.
- Implemented so far: profile loader, typed endpoint/receiver projection, PostgreSQL and Prometheus capability declarations, credential/TLS handling, runtime ownership wiring, profile diagnostics, profile tests, operator examples, and Windows live E2E. Linux exact-binary verification and isolated Docker/OTLP E2E remain mandatory before merge.

## Actual paths touched to date

`internal/monitoringprofile/{profile.go,loader.go,loader_test.go,docker_config.go,diagnostics_test.go}`, `internal/collectorruntime/{manifest.go,manifest_test.go}`, `cmd/agentctl/{native_monitoring_runtime.go,native_monitoring_runtime_test.go,bootstrap_command.go,reconciliation_runtime.go,template_materialization_runtime.go,inventory_command.go,application_profiles_live_e2e_test.go}`, `.github/workflows/ci.yml`, `configs/monitoring-profiles/`, and the local PR-05/PR-06 architecture and progress documents.

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
