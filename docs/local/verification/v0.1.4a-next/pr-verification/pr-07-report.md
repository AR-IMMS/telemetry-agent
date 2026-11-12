# PR-07 verification report — OpAMP Supervisor and control plane

**Status:** IN_PROGRESS — Windows and Ubuntu WSL live Supervisor/Collector
fallback, local/remote composition, rollback, Collector crash recovery, and
actual OTLP delivery passed. The pinned Linux Supervisor/Collector E2E also
passed on Ubuntu 24.04 WSL. A raw Supervisor kill/restart failed because the
Collector child remained alive; the coordinated case passed when the outer
manager terminated that child before restart. The Agent process-tree manager
is not implemented yet. Windows Docker profile parity passed. Linux Agent and
Supervisor profile parity also passed under Ubuntu WSL against Docker Desktop's
Linux Engine; native Linux Docker Engine parity remains unverified. PR-07b protocol and pinned Supervisor auth passed on both OSes
against the Go Mock Core. Production Core interoperability, actual Agent-owned
Supervisor lifecycle, and native Linux Docker parity remain open. Do not
transfer Collector lifecycle ownership on this evidence.
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
  they actually executed. Supervisor-process recovery depends on the outer
  Agent/service manager also cleaning up its child; both raw and coordinated
  cases are recorded below. See the [pinned Supervisor README](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.160.0/cmd/opampsupervisor/README.md)
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
4. Profile parity is verified on Windows and Ubuntu WSL for Redis, PostgreSQL,
   Python Prometheus, and a recreated cooling workload. WSL's Docker Engine
   reports `Docker Desktop (containerized)`; native Linux Docker Engine parity
   and Supervisor host-profile coverage remain open.
5. Collector crash/restart and Supervisor restart with explicit child cleanup
   passed on Windows and Linux. Raw Supervisor restart alone failed on Linux:
   the orphan Collector kept its OTLP and health listeners bound. Implement and
   verify Agent/service process-tree cleanup before lifecycle ownership transfer.
6. Ubuntu 24.04 WSL hostmetrics E2E and Docker workload/profile E2E passed
   using the pinned Linux binaries. Native Linux Docker Engine E2E remains
   BLOCKED because the available Docker endpoint is Docker Desktop.
7. Review migration/rollback so the Agent Runtime remains the sole owner until
   one reversible ownership transfer after parity.

## PR-07a verification log

| Gate | Status | Evidence / note |
|---|---|---|
| Exact Windows Supervisor artifact and checksum | PASS | Official v0.160.0 Windows amd64 artifact SHA-256 `df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449`; executable `--help` returned successfully. |
| Exact Linux Supervisor artifact and checksum | PASS | Official v0.160.0 Linux amd64 artifact SHA-256 `7967e3210e0252f25f41a96b15758dd2e03feb0aecbe85a945f1e2e6e71865c9`; executable on Ubuntu 24.04 WSL. |
| Collector v0.160.0 artifact hashes | PASS | Official Windows archive SHA-256 `d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf`; Linux archive SHA-256 `7bb60c584c241c86261c2b8697cd3725dd8c56691f5ad5d98454eaa005b47b0c`; both match repository manifests. |
| Pinned Supervisor/Collector interoperability | PASS / LIMITED | Exact pinned Supervisor and Collector delivered hostmetrics on Windows 11 and Ubuntu 24.04 WSL. Linux test used the native WSL2 Linux kernel; Docker workloads were not part of this test. |
| Local/remote config composition | PASS / LIMITED | Windows and Linux E2E composed a local `hostmetrics` pipeline with an OpAMP remote override of the OTLP exporter endpoint, restarted Collector, and observed output at the new isolated sink. Dynamic local-file update semantics remain NOT RUN. |
| Offline startup fallback and reconnect | PASS / LIMITED | Windows and Linux Supervisor started Collector from standalone fallback while OpAMP was unavailable, connected when the test server started, and applied remote config. Reconnect-after-established-session-loss remains NOT RUN. |
| Collector crash recovery | PASS / LIMITED | On Windows and Linux, test killed the managed Collector after successful config; Supervisor started a new Collector process and OTLP output resumed at sink B. |
| Supervisor process crash recovery | CONDITIONAL / OUTER-MANAGER REQUIRED | Raw Linux Supervisor kill/restart failed: the orphan Collector retained ports `127.0.0.1:37035` and `127.0.0.1:8888`, so its replacement could not start. Evidence: `tmp/pr07a/evidence/linux/supervisor-crash-orphan-recovery.txt`. Coordinated restart passed on Windows and Linux after explicitly terminating the child, modeling the required Agent/service process-tree manager. That manager is not yet implemented; keep lifecycle transfer blocked. |
| Invalid candidate rejection and recovery | PASS / LIMITED | A syntactically valid remote config using an already-bound OTLP receiver port caused activation failure and an OpAMP `FAILED` status. The test server restored the prior config; the Supervisor reported that hash `APPLIED` and sink B delivery resumed. |
| Automatic config rollback / Agent LKG parity | PASS / LIMITED | With the desired-state test server sending only changed config hashes, the pinned Supervisor reported `FAILED` for an activation-failing config, automatically restored the previous working remote config, and emitted `APPLIED` for its hash within the 20-second observation window. OTLP resumed. The test server had to avoid replaying the same failed hash. Agent Runtime LKG parity remains NOT RUN. |
| Actual OTLP delivery | PASS / LIMITED | Windows and Linux hostmetrics reached isolated sink A during fallback and sink B after remote update, activation recovery, Collector crash restart, and coordinated Supervisor process restart. Redis, PostgreSQL, Python Prometheus, and mock-cooling profiles reached the isolated sink under Windows Supervisor fallback and OpAMP reconnect. |
| Existing application-profile parity | PASS / LIMITED | Windows Docker E2E and Ubuntu WSL/Docker Desktop E2E reused PR-06 Redis, PostgreSQL, Python Prometheus, and mock-cooling profiles. Agent Runtime emitted all four streams; after stopping it, Supervisor emitted the same profiles from startup fallback and after OpAMP reconnection. Container recreation delivered the new cooling `resource_id`; tests found no Agent-created duplicate receivers or duplicate OTLP samples. Native Linux Docker Engine parity remains BLOCKED; hostmetrics was covered by the separate Supervisor/Collector E2E. |
| Windows native E2E | PASS / LIMITED | Focused Windows live test with exact binaries, local `opamp-go` server, and two isolated Collector OTLP sinks. This is a smoke test, not complete PR-05/06 parity. OS build number was not captured. |
| Native Linux Supervisor/Collector/OTLP E2E | PASS / LIMITED | Ubuntu 24.04 WSL2 kernel `6.18.58.1-microsoft-standard-WSL2`, pinned Linux Supervisor and Collector, local OpAMP test server, and isolated OTLP sinks. Full Linux test/vet/build results are recorded below. |
| Linux Docker profile parity | PASS / WSL + DOCKER DESKTOP | Ubuntu WSL Go Agent and pinned Linux Supervisor/Collector exercised Redis, PostgreSQL, Python Prometheus, mock cooling, container recreation, profile identity, no duplicate samples, startup fallback, reconnect, and OTLP delivery against Docker Engine 29.5.3. `docker info` identifies `Docker Desktop (containerized)`; this does not verify a native Linux Docker Engine. Evidence: `tmp/pr07a/evidence/linux/application-profiles/`. |
| Native Linux Docker Engine parity | BLOCKED | No native Linux Docker Engine is available in this environment; the available daemon is Docker Desktop. Run the same test on a native Linux Docker host before treating platform parity as complete. |
| Mock Core contract interoperability | PR-07B | See the PR-07b local contract and protocol verification section below. |
| Production NestJS Core interoperability | BLOCKED | Core control operations are absent from inspected API baseline; requires reviewed contract and a separate Core implementation. |
| Lifecycle ownership transfer / rollback | BLOCKED | Must wait for 07a parity and 07b contract gates; no transfer occurred. |

## PR-07b local contract and protocol verification

| Gate | Status | Evidence / note |
|---|---|---|
| Local versioned Core control contract | PASS / MOCK-BACKED | `docs/local/architecture/v0.1.4a-next/contracts/agent-control-v1.openapi.yaml` defines only authenticated session/policy read, first-bind compare-and-set, and status report. It is not published to `docs/shared/` and is not a production Core contract. |
| Existing enrollment bearer and Core response conventions | PASS / MOCK-BACKED | TLS `httptest` integration exercises HTTPS-only client, existing bearer, `{data,meta.request_id}` responses, RFC problem responses, revocation, binding conflict, and identity mismatch. Duplicate identical status PUTs are idempotent in Mock Core. |
| OpAMP server policy and status translation | PASS / MOCK-BACKED | Live `opamp-go` v0.23.0 WebSocket client connects to the Go adapter and HTTPS Mock Core, receives the versioned Gateway policy, binds its instance UID, and reports APPLIED revision 1. See `go test ./internal/opampserver -run '^TestLiveAuthenticatedSupervisorToMockCorePolicyAndStatus$' -count=1 -v`. |
| Pinned Supervisor bearer-auth config | PASS / WINDOWS + UBUNTU WSL | Exact Supervisor/Collector v0.160.0 with `server.auth: bearertokenauth` and the `opampsupervisor.Extensions` gate connected to the adapter; Mock Core recorded the bound instance and APPLIED revision 1 on both operating systems. The isolated test disables Collector internal metrics to avoid colliding with an existing `localhost:8888` listener and sets a 15s Collector bootstrap timeout. Evidence: `tmp/pr07b/evidence/{windows,linux}/status-summary.txt` and `supervisor.log`. |
| Remote config status attribution safeguards | PASS / WINDOWS + UBUNTU WSL | Unknown hashes are not reported as the current desired revision; retained hash-to-revision history is bounded to 64 instances and 32 hashes per instance. The live `opamp-go` E2E advances policy revision 1→2 without changing endpoint/config hash and verifies APPLIED revision 2. The map is process-local, so attribution for older pre-existing hashes after adapter restart remains unverified and will be suppressed until observed again. |
| PR-07b Go package regression | PASS / WINDOWS | `go test ./internal/coreapi ./internal/opampserver ./internal/mockcore ./cmd/agentctl -count=1` passed through the authorized Windows environment. Sandbox-local TLS loopback execution is denied by OS socket permissions; that failed invocation is environmental, and the same suites passed in the authorized environment. |
| Repository Go regression, vet, and builds | PASS / WINDOWS + UBUNTU WSL | `go test ./... -count=1`, `go vet ./...`, and `go build ./...` passed on Windows and Ubuntu 24.04 WSL with Go 1.25.5. WSL run uses `GOMODCACHE=/tmp/codex-pr07-gomodcache`, `GOCACHE=/tmp/codex-pr07-gocache`; this verification also confirmed test-only PR-07a helper files compile on both operating systems. |
| Production NestJS Core interoperability | BLOCKED | Inspected Core API Baseline contains no Agent control routes. Local Mock Core is replaceable by endpoint configuration but does not prove production interoperability. |
| Supervisor credential provisioning by Agent | NOT RUN | The live test supplies a short-lived test bearer through a protected temp token file. No Agent installation/provisioning/rotation behavior changed in PR-07b. |
| Native Linux Supervisor/Mock Core E2E | PASS / MOCK-BACKED | Ubuntu 24.04 WSL native Go OpAMP test and exact pinned Linux Supervisor v0.160.0 bearer-auth test passed against HTTPS Mock Core; sanitized evidence under ignored `tmp/pr07b/evidence/linux/`. This does not prove NestJS Core interoperability. |

The pinned Windows and WSL Supervisor tests retain sanitized logs and a status
summary under ignored `tmp/pr07b/evidence/{windows,linux}/`. The token is
synthetic, stored in a temporary token file, and redacted from retained logs.
The first Windows rerun exposed a real testbed collision with the Collector's
default `localhost:8888` metrics listener; the isolated harness now disables
that internal metrics reader. This is a harness setting, not a production
Collector configuration change.

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
- Linux Supervisor E2E passed on 2026-11-13 on Ubuntu 24.04 WSL using pinned
  Linux Supervisor/Collector binaries and isolated local OTLP sinks. The
  `tmp/pr07a/evidence/linux/` directory contains `test-summary.txt`, process
  output and sink logs. `supervisor-crash-orphan-recovery.txt` preserves the
  separate raw-restart failure that showed why Agent/service process-tree
  cleanup is mandatory.
- The Linux Mock Core live run passed on 2026-11-13. Sanitized status and
  Supervisor logs are under ignored `tmp/pr07b/evidence/linux/`.
- The test-only harness does not change production installation, runtime
  ownership, config, or lifecycle behavior. The pre-existing untracked
  `cmd/agentctl/otel.yaml.lifecycle.lock` remains preserved and excluded.

## Merge audit requirement

Before merging a checkpoint, tie evidence to the exact commit and platform. Run
CI after final code/docs/report edits; record environment versions, failure
injection, single-owner/rollback result, and sink evidence. Keep mandatory
gates open and the checkpoint draft while any are unresolved.
