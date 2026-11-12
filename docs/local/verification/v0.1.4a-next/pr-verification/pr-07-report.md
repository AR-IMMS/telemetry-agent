# PR-07 verification report — OpAMP Supervisor and control plane

**Status:** IN_PROGRESS — Windows Supervisor v0.160.0 fallback, local/remote
composition, APPLIED status, activation failure, automatic rollback, Collector
crash recovery, and actual OTLP delivery passed. Windows Docker profile parity
also passed for Redis, PostgreSQL, Python Prometheus, and a recreated cooling
workload. Supervisor-process crash recovery and native Linux gates remain open.
Do not merge or transfer Collector lifecycle ownership on this evidence.
**Release target:** `v0.1.4a-next`
**Branch:** `feat/pr-07-opamp`
**Implementation baseline:** PR-06 merge
`af2f5db4c6b8832bc7bcc87d332d899dad01b452`.

## Target change reviewed

The accepted target is staged: (07a) exact-version Supervisor capability spike
without ownership transfer; (07b) reviewed versioned contracts plus Go OpAMP
server and Go Mock Core; (07c) controlled runtime adoption after parity. The
Agent Runtime remains the only production Collector lifecycle owner throughout
07a/07b. Production Core changes and shared-contract publication are outside
this implementation branch's authorized scope.

## Verified documentation and current-state facts

- Existing `internal/supervisor` and `internal/agentlifecycle` own the current
  Agent Collector process/runtime path. They are distinct from OTel OpAMP
  Supervisor, and the production runtime was not modified in PR-07a.
- Authoritative enrollment OpenAPI blob
  `5ce0f6a3d94401c2adf65eaf9ad524a37b868571` and Core `api/baseline.yaml` blob
  `4f4166f2a4d1770f5142618d58b1997092d289c0` were fetched from the user-named
  repositories. Enrollment uses a single-use bootstrap token with a 10-minute
  initial window and bounded same-attempt credential recovery; it does not
  define an Agent bearer-authenticated control endpoint. Core reserves the
  `agentBearer` scheme, but the baseline defines no Agent control operation.
  PR-07b must define the smallest supported control API contract without
  changing enrollment semantics. Production Core interoperability remains
  unverified.
- Official Supervisor v0.160.0 docs describe remote configuration as opt-in,
  merged with local config, and applied through Collector restart. Startup
  fallback is standalone; automatic rollback is opt-in and described for
  Collector startup failure. The Windows tests below confirm only the scenarios
  they actually executed; Supervisor-process recovery and Linux behavior remain
  unverified. See the [pinned Supervisor README](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.160.0/cmd/opampsupervisor/README.md)
  and [pinned specification](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.160.0/cmd/opampsupervisor/specification/README.md).

## Contract and compatibility gates still open

1. Define installation/binding ↔ OpAMP `instance_uid` identity and authenticated
   Supervisor credential provisioning, storage, rotation, revocation, and
   session invalidation. `instance_uid` is not authentication.
2. Define schema/version, target, desired/applied revision, hash canonicalization,
   local/remote field ownership, merge order, deletion and stale/conflict rules
   for the Gateway endpoint policy.
3. Map protocol/runtime evidence to truthful received, validated, applying,
   applied, failed, recovered and unknown statuses. Readiness is not scrape or
   OTLP delivery evidence; an older applied hash after rollback is not success
   for a newer desired revision.
4. Profile parity is verified on Windows for Redis, PostgreSQL, Python
   Prometheus, and a recreated cooling workload; verify the same on native Linux
   and add host-profile coverage under Supervisor.
5. Induce invalid remote candidate, Collector crash/restart, Supervisor restart,
   fallback recovery, and automatic rollback; compare behavior with Agent LKG.
6. Verify native Linux execution, not only Linux artifact checksum or a Windows
   host running Linux containers.
7. Review migration/rollback so the Agent Runtime remains the sole owner until
   one reversible ownership transfer after parity.

## PR-07a verification log

| Gate | Status | Evidence / note |
|---|---|---|
| Exact Windows Supervisor artifact and checksum | PASS | Official v0.160.0 Windows amd64 artifact SHA-256 `df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449`; executable `--help` returned successfully. |
| Exact Linux Supervisor artifact and checksum | PASS | Official v0.160.0 Linux amd64 artifact SHA-256 `7967e3210e0252f25f41a96b15758dd2e03feb0aecbe85a945f1e2e6e71865c9`. Native execution remains NOT RUN. |
| Collector v0.160.0 artifact hashes | PASS | Official Windows archive SHA-256 `d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf`; Linux archive SHA-256 `7bb60c584c241c86261c2b8697cd3725dd8c56691f5ad5d98454eaa005b47b0c`; both match repository manifests. |
| Pinned Supervisor/Collector interoperability | PASS / LIMITED | Windows Supervisor managed the exact pinned Windows Collector and delivered metrics for the tested hostmetrics config. Linux interoperability remains NOT RUN. |
| Local/remote config composition | PASS / LIMITED | Windows E2E composed a local `hostmetrics` pipeline with an OpAMP remote override of the OTLP exporter endpoint. Collector restarted, then reported the matching remote config hash APPLIED. Dynamic local-file update semantics remain NOT RUN. |
| Offline startup fallback and reconnect | PASS / LIMITED | Windows Supervisor started Collector from standalone fallback while OpAMP was unavailable, connected when the test server started, and applied remote config. Reconnect-after-established-session-loss remains NOT RUN. |
| Collector crash recovery | PASS / LIMITED | Test killed the managed Collector after successful config; Supervisor started a new Collector process and OTLP output resumed at sink B. Supervisor process crash recovery remains NOT RUN. |
| Invalid candidate rejection and recovery | PASS / LIMITED | A syntactically valid remote config using an already-bound OTLP receiver port caused activation failure and an OpAMP `FAILED` status. The test server restored the prior config; the Supervisor reported that hash `APPLIED` and sink B delivery resumed. |
| Automatic config rollback / Agent LKG parity | PASS / LIMITED | With the desired-state test server sending only changed config hashes, the pinned Supervisor reported `FAILED` for an activation-failing config, automatically restored the previous working remote config, and emitted `APPLIED` for its hash within the 20-second observation window. OTLP resumed. The test server had to avoid replaying the same failed hash. Agent Runtime LKG parity remains NOT RUN. |
| Actual OTLP delivery | PASS / LIMITED | Windows hostmetrics reached isolated sink A during fallback, sink B after remote exporter update, and sink B again after failure recovery and Collector crash restart. Redis, PostgreSQL, Python Prometheus, and mock-cooling profiles also reached the isolated sink under Supervisor fallback and after OpAMP reconnect. |
| Existing application-profile parity | PASS / LIMITED | Windows Docker E2E reused PR-06 Redis, PostgreSQL, Python Prometheus, and mock-cooling profiles. Agent Runtime emitted all four streams; after stopping it, Supervisor emitted the same profiles from startup fallback and after OpAMP reconnection. Docker container recreation delivered the new cooling `resource_id`. The candidate had no Agent-created duplicate receiver and all observed samples reached the isolated OTLP sink. Native Linux and hostmetrics in the profile-parity run remain NOT RUN. |
| Windows native E2E | PASS / LIMITED | Focused Windows live test with exact binaries, local `opamp-go` server, and two isolated Collector OTLP sinks. This is a smoke test, not complete PR-05/06 parity. OS build number was not captured. |
| Native Linux E2E | NOT RUN | No native Linux runner used. |
| Mock Core contract interoperability | NOT RUN | Not implemented or tested. |
| Production NestJS Core interoperability | BLOCKED | Core control operations are absent from inspected API baseline; requires reviewed contract and a separate Core implementation. |
| Lifecycle ownership transfer / rollback | BLOCKED | Must wait for 07a parity and 07b contract gates; no transfer occurred. |

## Reproduction and retained evidence

- Focused test:
  `go test ./cmd/agentctl -run '^TestPR07ASupervisorRemoteConfigAndFallbackDelivery$' -count=1 -v`
- Windows Docker profile parity:
  `go test ./cmd/agentctl -run '^TestLiveDockerApplicationProfilesDeliverOTLP$' -count=1 -v`
  with `AR_IMMS_PR06_APPLICATION_PROFILE_E2E=1`,
  `AR_IMMS_PR07A_SUPERVISOR_PROFILE_PARITY=1`, verified Supervisor/Collector
  binary paths, and `AR_IMMS_PR07A_EVIDENCE_DIR=<directory>`.
- Variables: `AR_IMMS_PR07A_LIVE_E2E=1`,
  `AR_IMMS_PR07A_SUPERVISOR_BINARY=<verified Windows Supervisor>`,
  `AR_IMMS_PR05_COLLECTOR_BINARY=<verified Windows Collector>`, and
  `AR_IMMS_PR07A_EVIDENCE_DIR=<directory>`.
- The passing run reported remote config SHA-256
  `a1547e7f8bbf20001b4c9ec665a1ae88f2f4a8fa35955dae99cda930ea543c9f` and
  delivered `system.memory.usage` from Collector Contrib `0.160.0` to both
  endpoints. Raw captured output is retained locally under ignored
  `tmp/pr07a/evidence/windows/` (`supervisor.log`, both sink logs, and
  `test-summary.txt`); re-running regenerates it. Verified downloads and
  extracted binaries are under ignored `tmp/pr07a/artifacts/`.
- `go test ./cmd/agentctl -run '^TestPR07A' -count=1` passed on Windows after
  the addition of the executable harness.
- The Windows Docker profile-parity run passed on 2026-11-13 with Redis,
  PostgreSQL, Python Prometheus, mock-cooling churn, offline startup fallback,
  OpAMP reconnect, and OTLP sink verification. Reproducible local evidence is
  retained under ignored `tmp/pr07a/evidence/windows/application-profiles/`
  (`supervisor.log`, `otlp-sink.log`, and `profile-parity-summary.txt`). Docker
  was accessed through the elevated Windows test environment; this is not
  native Linux verification.
- The test-only harness does not change production installation, runtime
  ownership, config, or lifecycle behavior. The pre-existing untracked
  `cmd/agentctl/otel.yaml.lifecycle.lock` remains preserved and excluded.

## Merge audit requirement

Before merging a checkpoint, tie evidence to the exact commit and platform. Run
CI after final code/docs/report edits; record environment versions, failure
injection, single-owner/rollback result, and sink evidence. Keep mandatory
gates open and the checkpoint draft while any are unresolved.
