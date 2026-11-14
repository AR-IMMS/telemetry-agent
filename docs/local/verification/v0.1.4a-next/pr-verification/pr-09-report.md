# PR-09 Controlled Operations Verification

- **Status:** IN_PROGRESS — Phase 1 Windows gates pass; native Linux live verification is pending exact-head CI. Phase 2 is not approved or enabled.
- **Branch:** `feat/pr-09-controlled-operations`
- **Base:** PR-08 merge `d1429196e8b39a24a624a9ad30619476f09a0a9f`
- **Release plan:** [PR-09 controlled operations](../../../plans/v0.1.4a-next/pr-09-controlled-operations.md)
- **Implementation reference:** User-provided `pr-09-detail-controlled-operations.md` from the original checkout.
- **Contracts:** Agent-local `ar-imms.operation.request.v1` and `ar-imms.operation.result.v1`; production Core interoperability remains separately tracked.

## Scope delivered

Phase 1 adds a bounded read-only Operations context. `internal/operations` owns versioned request/result models, typed handlers, registry, envelope validation, canonical request fingerprints, and execution dispatch. The handlers reuse existing Health, Docker, and managed-dependency evidence. `internal/operations/sqlite` adds migration v14 for execution state, append-only audit events, and a transactional result outbox. `internal/operations/nats` uses authenticated TLS JetStream pull consumption and PubAck-based outbox delivery. The test-only Mock Core provisions and consumes the scoped test messages. Optional Operations composition is isolated from telemetry lifecycle when broker configuration or service is unavailable.

The only registered operation types are `agent.diagnostics`, `container.inspect`, and `dependency.inspect`. Container inspection requires the full exact container ID and returns an allowlisted projection; it does not expose environment, labels, mounts, or raw Docker SDK data. A repeated operation ID with a different request fingerprint is rejected. A duplicate with the same fingerprint reuses the stored terminal result without invoking its handler again. The Operations runner acquires an installation-scoped process lock before startup recovery and command consumption. Interrupted read-only claims are finalized as `FAILED` / `EXECUTION_INTERRUPTED`, audited, and put in the result outbox; handlers are not silently retried while retry semantics are unapproved. Handler execution is bounded to 30 seconds or request expiry, and ACK progress is extended while a handler runs.

Core remains the business authorization authority. `requested_by` and `authorization_ref` are retained as bounded audit correlations only. The Agent checks TLS broker scope, installation/binding identity, expiry, operation allowlist, and exact targets. These local contract files do not supersede or modify `docs/shared/` or provide production Core authorization.

## Verification

| Gate | Result | Evidence / limitation |
|---|---|---|
| Operations, handler, SQLite, NATS adapter and Docker provider tests | PASS | `go test ./internal/operations/... ./internal/discovery/docker -count=1` with `TEMP` and `TMP` under `.local-verification/pr09-tmp`. |
| Native Windows live JetStream + Docker E2E | PASS | Fresh run at HEAD `e359bc389cdea04377ca9f52903b7ca8f7923c06`: `go test ./internal/operations/nats -run '^TestLiveJetStreamReadOnlyOperationsE2E$' -count=1 -v`, elevated Docker Desktop access, exit 0 (32.12s). Exercises queued delivery through a JetStream server restart while Agent is offline, exact live Docker inspection, the three-operation dispatch, fixture-secret exclusion, distinct transport-ID replay, Agent/Core reconnect after a second server restart, durable SQLite audit/outbox, and denials for cross-installation publishing, Agent command publication, unauthorized consumers, and stream administration. Retained [live output](pr-09-windows-live-output.txt), SHA-256 `3FB1706B186C4A7A41EB8061EE6FCC9D944BB278CC3E1062533E9E8C0E11F23F`. Permission-denial log lines are expected assertions. `dependency.inspect` dispatch crosses real JetStream but currently uses an injected inspector; native managed-dependency status remains unverified. |
| Native Windows full Go suite | PASS | `go test ./... -count=1` with workspace temp directory and permissions for loopback tests; every package reported PASS, exit 0. Retained [full test output](pr-09-windows-full-test-output.txt), SHA-256 `D3FD87C19C57453697DCF5826CE59386F57F2C9E06146B21C4F6DB60654402DE`. An initial restricted run failed unrelated loopback and Windows Exporter tests due sandbox socket/probe restrictions; the permitted rerun passed. |
| Native Windows vet, build, architecture guard | PASS | `go vet ./...`, `go build ./...`, and `./scripts/check-architecture.ps1`; all exit 0. The guard passed package docs/import boundaries and the production file-size check. |
| Native Linux live E2E | VERIFYING | Added an exact live E2E step to the existing Ubuntu job after its isolated Docker-in-Docker daemon is available. The test fixture uses `docker cp` rather than a daemon-host bind mount so it works with the remote Docker Engine. Exact-head CI has not yet run. |
| Native Linux full suite, vet, build, architecture guard | VERIFYING | Existing CI steps execute these gates on Ubuntu; require exact-head run results. |
| Production NestJS Core / broker provisioning interoperability | NOT_TESTED | This repository does not own the Core implementation or approved broker-provisioning contract. Mock Core is the approved implementation checkpoint. |
| Real managed dependency inspection, expired/wrong-binding delivery, credential replacement/revocation, Agent process restart, broker outage after result commit, malformed-message exhaustion | NOT_TESTED | Focused validation/recovery tests cover selected local semantics, but these cases are not covered by the live E2E and remain required follow-up before final release acceptance. |
| `container.restart`, mutation recovery, sensitive logs | DEFERRED | No operation-specific safety/exposure contract approval; no mutation handler is registered or remotely reachable. |

The Windows E2E starts a pinned real NATS Server image (`nats:2.15.3-alpine`, digest pinned in the test), provisions distinct `mock-core` and Agent identities, and verifies denied cross-installation result publication and unauthorized durable access. The test validates real Docker inspection and JetStream delivery, but its managed dependency inspector is a test implementation; production adapter wiring delegates to the existing `dependency.Service.Status` and requires platform-specific managed dependency evidence in a broader runtime E2E.

## Historical preimplementation audit

The initial source audit at PR-08 merge `d1429196e8b39a24a624a9ad30619476f09a0a9f` found no published Operations dispatch or result contract. The [enrollment contract](https://github.com/AR-IMMS/shared-docs/blob/main/contracts/openapi/agent-enrollment.openapi.yaml) explicitly did not define control-plane operations, and the [Core API baseline](https://github.com/AR-IMMS/core-backend/blob/dev/api/baseline.yaml) had no API paths. The former readiness-only report marked PR-09 BLOCKED because no contract was then approved. This checkpoint proceeds with the user's approved Agent-local Phase 1 design; the findings remain relevant to production Core interoperability and do not authorize Core-side changes.

## Open decisions and follow-up

1. Approve and publish cross-repository Core/broker credential provisioning, identity-binding, and result acceptance contracts before claiming production interoperability.
2. Approve operation-specific `container.restart` policy before Phase 2: explicit opt-in, target freshness, restart eligibility, deadlines, idempotency, pre/post evidence, and unknown-outcome recovery. Until then, the mutation is absent from the registry and schemas.
3. Keep Windows and Linux evidence separate. Linux verification must execute the real live JetStream/Docker test, not rely on cross-compilation or skipped tests.
4. Review result/audit retention bounds with Core's replay duration before release acceptance; the current implementation retains SQLite execution identity and uses bounded NATS stream retention configured by infrastructure.

## Files and rollback

Agent-local JSON schemas, transport contract, and ADR document the Phase 1 boundary. Local architecture and release-progress documents now identify the implemented context and current gate state. No `docs/shared/`, Core production code, collector behavior, or CLI surface is changed by this checkpoint. SQLite migration v14 is additive. Rollback is to stop provisioning Operations credentials and remove the optional runtime configuration; existing telemetry and lifecycle remain independent. A schema downgrade is not supported or required.

**Next checkpoint:** Complete exact-head Ubuntu CI and review its retained live E2E artifact. After PR-09 Phase 1 is verified/merged, begin PR-09.5's behavior-preserving ownership audit; keep Phase 2 disabled until its contract is explicitly approved.
