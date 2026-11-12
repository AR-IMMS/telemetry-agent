# PR-07 — NestJS OpAMP Server Adapter & Go Agent Client

**Status:** PRE-IMPLEMENTATION PLAN / implementation not started. **Release target:** `v0.1.4a-next`. **Evidence:** required from actual target-repository HEAD, not assumed.

## Objective and exit boundary

Deliver remote config via OpAMP transport (initially HTTPS plain HTTP polling) from Core NestJS to Agent Go, with durable desired/effective hashes and truthful status. Do not implement a generic remote command channel here.

## Baseline / current architecture constraints

Core enrollment and Agent identity/credential already exist. Current accepted Gateway snapshot != a fleet-wide remote policy system. OpAMP server must be implemented/verified against protocol specification, not guessed protobuf fields.

## Affected contexts and anticipated code areas

Use `docs/local/architecture/v0.1.4a-next/context-boundaries.md` and `source-structure.md` as canonical target. Source mapping is refined against current HEAD during step 1. Expected contexts: see objective and migration roadmap.

## Implementation steps (ordered)

1. Before writing protocol code, record an ADR on **minimum** policy authority: scope of writable fields, local bootstrap retention, targeted Core revision, and invalid/older/conflicting revision behavior; defer broad policy UX, not conflict safety.
2. Define server-side OpAMP Agent association: credential to installation to registered Node; `instance_uid` is protocol identifier and is not authentication. Map UID lifecycle explicitly.
3. Generate Protobuf types from one pinned official OpAMP schema and implement supported HTTP binary transport + message validation in NestJS adapter; constrain size, TLS/auth, rate and session replay.
4. Implement Go Agent OpAMP client with reconnect/backoff and explicit capabilities; persist accepted transport payload/hash separately from effective Collector config/generation in SQLite.
5. Use an AR-IMMS versioned monitoring policy file inside OpAMP `config_map`; validation -> configuration desired revision -> existing reconcile -> runtime activation/LKG.
6. Report `RemoteConfigStatus` based on protocol-defined statuses and actual runtime outcome; health/effective config optional by declared capability; not treat ready as delivered.
7. Add Core admin API integration and status display/records; scope config to authorized node and never send arbitrary executable shell/OTel YAML.
8. Implement hard offline/reconnect/reboot, bad config, same hash/no-op, revoked credential and rollback tests.

## Edge cases and failure semantics

- A valid config hash can still fail compile/activate: report FAILED and keep LKG.
- Multiple Agent instances/reconnect cannot cross-deliver policy between nodes.
- Legacy local config and Core config precedence unresolved: minimally deterministic and documented rule is required before implementation.
- TLS renewal, lost HTTP response, duplicate sequence/hash, malformed protobuf, oversize payload, unknown fields/version.
- A pending config must not be misreported APPLIED until runtime readiness; no claim backend delivery.

## Acceptance / verification gates

- Cross-repository NestJS↔Go native Linux/Windows E2E with actual OpAMP HTTP Protobuf exchanges.
- Targeted config updates only intended Node; hash/status/state after restart and reconnect.
- Invalid/revoked/expired requests rejected; offline continues LKG; same hash no generation bump.
- Protocol compliance tests for framing/content-type/capabilities and status mapping; Core IAM/audit checks.

**Evidence to attach:** before/after state and test commands; platform and Docker/Collector versions, native metrics or agent/service logs where relevant; Git SHA; unreached items explicitly NOT RUN/BLOCKED. Tests must reproduce *negative* paths as well as happy paths.

## Rollback and stop conditions

Disable OpAMP consumption and restore last effective local config; retain enroll identity and durable last-known-good. Core server endpoint may be disabled without disturbing OTLP.

**Stop/seek architecture review** if a new protocol interpretation, security elevation, state migration, new OS service ownership, unsupported OTel component, or config authority conflict is required. No silent scope expansion.

## Mandatory delivery contract

- **Before coding:** check current `git HEAD`, clean/dirty working tree including submodule, exact package paths, implemented behavior, Collector v0.160.0 **actual binary manifest**, and previous PR evidence. The paths below are *proposed ownership targets*, not confirmed post-refactor filenames. Do not infer functionality from package names.
- **Each PR delivers:** implementation plan updated with actual touched paths; code + focused tests; `pr-verification/pr-XX-report.md` recording commit, environment, commands, observed outputs, PASS/FAIL/BLOCKED/NOT RUN, risk, follow-up and rollback.
- **Shared invariants:** Windows/Linux binaries and services, CLI behavior, SQLite migration/CAS, enrollment/credential secrecy, one active receiver owner, Collector generation/readiness/LKG, managed-dependency ownership and real OTLP metrics. Never mark a simulated or mocked test as a live E2E PASS.
- **Quality:** `gofmt`, `go test ./...`, `go vet ./...`, focused `go test -race` where supported; both OS build gates; architectural import check, production Go file ≤450 LOC; per-feature real-host tests. Core NestJS changes use lint/typecheck/unit/integration tests in Core repository. Record pre-existing failures separately.
