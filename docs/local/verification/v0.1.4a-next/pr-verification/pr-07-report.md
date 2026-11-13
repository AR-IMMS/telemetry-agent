# PR-07 verification report — OpAMP Supervisor and control plane

**Status:** IN_PROGRESS — PR-07c connects pinned Supervisor installation,
protected credential/config/state provisioning, opt-in single-owner runtime
selection, credential-reference restart handling, and Windows Job Object/Linux
process-group containment. Final-head CI run
[`43400294752`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43400294752)
passed Windows and Ubuntu at `d6729066668e7f49797ee39c78b55623e5c284be`.
Both platforms executed full tests, Agent-managed enrollment, authenticated
WSS, remote Gateway revisions 1 and 2 with delivery to both isolated sinks,
`go vet`, and `go build`. Ubuntu additionally passed isolated native Docker
Redis/Collector E2E and the explicitly enabled Supervisor profile fallback,
reconnect, and OTLP parity subcase for Redis, PostgreSQL, Prometheus, and mock
cooling; profile and managed-runtime evidence artifacts were uploaded. Corrected
final-head run `43405983503` passed the expanded managed E2E on native Windows
and Ubuntu: enrollment, authenticated WSS, credential replacement and old-token
rejection, reconnect, policy revisions 1–3, and OTLP at three distinct Gateway
sinks. Ubuntu also passed Agent-managed Docker profile parity for Redis,
PostgreSQL, Python Prometheus, and mock cooling, including container churn and
duplicate-sample checks. Durable Mock Core hash/revision resolution across Core
restart, a fresh OpAMP/Agent runtime, and revision-5 prior-hash rollback passed
on both native platforms. Agent-managed offline fallback followed by authenticated
OpAMP reconnect also passed. Remaining gates: local config update/no-op; managed
Supervisor/Collector crash and Agent/service recovery; LKG equivalence; Windows
managed-profile E2E; and native Windows/Linux service install, migration,
upgrade, and uninstall. Keep Supervisor opt-in and PR #30 draft until all
mandatory adoption gates pass.
**Release target:** `v0.1.4a-next`
**Branch:** `feat/pr-07-opamp`
**Implementation baseline:** PR-06 merge
`af2f5db4c6b8832bc7bcc87d332d899dad01b452`.
**Expanded credential-rotation test commits:** `882e450` adds the rotation path;
`261d332` corrects post-rotation delivery to a distinct third Gateway;
`1e7c23d` moves the profile parity subcase under Agent-managed Supervisor
ownership. Final-head CI run `43405983503` passed both platform jobs.

## Target change reviewed

The accepted target is staged: (07a) exact-version Supervisor capability spike
without ownership transfer; (07b) reviewed versioned contracts plus Go OpAMP
server and Go Mock Core; (07c) controlled runtime adoption after parity. The
Agent Runtime remains the only production Collector lifecycle owner throughout
07a/07b. Production Core changes and shared-contract publication are outside
this implementation branch's authorized scope.

## PR-07c implementation progress

| Slice | Result | Evidence / limit |
|---|---|---|
| Pinned Supervisor artifact selection | IMPLEMENTED / WINDOWS UNIT TESTS | `internal/bootstrap/supervisor_artifact.go` selects official v0.160.0 Windows/Linux amd64 release binaries and the digests already verified by the PR-07a spike. Unsupported platforms fail closed. |
| Staged installation and reuse | IMPLEMENTED / WINDOWS UNIT TESTS | `InstallSupervisorBinary` bounds download size, verifies SHA-256 before atomic publication, stores version/platform/digest metadata, re-hashes installed bytes before reuse, and refuses to overwrite an unverified version directory. Repro: `go test ./internal/bootstrap -run 'Test(SelectSupervisorArtifact|InstallSupervisorBinary)' -count=1`. |
| Protected token/config/state provisioning | IMPLEMENTED / LOCAL WINDOWS ELEVATION REQUIRED | `credentialstore.Store` materializes the existing enrollment bearer to a fixed protected file, writes typed allowlisted Supervisor YAML, and prepares persistent Supervisor storage under the protected directory. Unit/integration tests cover token replacement, missing credentials, cleanup, configuration rendering, and path/mode rules. Local Windows filesystem tests skip because the current process is not elevated. Live auth, rotation/revocation, and uninstall cleanup remain unverified. |
| Packaged installer integration | IMPLEMENTED / FOCUSED WINDOWS TESTS | The installer invokes pinned Supervisor provisioning only for explicit `--opamp-server-url` migration or when preserving a previously enabled Supervisor installation. Fresh installs without the option retain the current Agent Collector owner; reinstall preserves the persisted owner. Endpoint validation rejects remote cleartext WS and invalid URLs. Native install/upgrade/uninstall service checks remain open. |
| Agent-owned process containment | IMPLEMENTED / WINDOWS TEST PASS; LINUX CROSS-COMPILE | `internal/processowner` uses a Windows suspended process plus kill-on-close Job Object and Linux process groups. Windows Job cancellation and descendants tests pass in the full suite. A Linux process-group integration test is present and its test binary cross-compiles, but it has not run on Linux in this environment. |
| Supervisor runtime and Agent ownership selection | IMPLEMENTED / LIVE CI GATE PENDING | Persisted `SupervisorContext` selects either the legacy Agent Collector runtime or `runManagedSupervisor`, never both. The managed runner provisions current enrollment credentials, writes protected config/state, supplies the Gateway endpoint, waits for Collector health before existing readiness/LKG callback, watches credential-reference changes, and contains process cleanup. Full live parity, crash recovery, service lifecycle, LKG rollback equivalence, and no-duplicate-sink proof remain open. |
| Agent-managed Supervisor fallback + OTLP E2E | PASS / LIMITED — WINDOWS + NATIVE UBUNTU CI | `TestManagedSupervisorRuntimeDeliversOTLP` checks the pinned Supervisor executable SHA-256 and Collector v0.160.0 version, validates and runs local hostmetrics fallback, waits for Collector health and `system.cpu.time`/`system.memory.usage` at an isolated OTLP sink, then cancels the contained process tree. It passed on both platforms in final-head run `43373790016`. The earlier failures were caused by both isolated Collectors binding `localhost:8888`; setting `service.telemetry.metrics.level: none` in both test configs resolved the collision. This test does not exercise Agent enrollment, authenticated OpAMP delivery, remote Gateway changes, credential lifecycle, or managed profile parity. |
| Durable publication and truthful rollback attribution | IMPLEMENTED / WINDOWS MOCK CORE TESTS | The local contract records exact policy revision/hash publications before OpAMP delivery and resolves observed hashes through Core. Desired, published, and applied revisions are distinct; old applied hashes remain rollback evidence. Mock Core's atomic JSON fixture survives restart without storing bearer values. The full Windows repository suite passed in this run; native Linux Mock Core execution remains NOT RUN. |
| Enrollment-to-WSS managed Gateway vertical slice | PASS / WINDOWS + NATIVE UBUNTU CI; RESTART EXTENSION PENDING | Run `43405983503` passed actual enrollment, protected credential storage, authenticated WSS, remote Gateway revisions 1–3, explicit re-enrollment, old credential rejection, protected replacement credential use, reconnect, and post-rotation OTLP. A local follow-up extends the same test to persist/reload Mock Core, resolve the applied revision-3 hash after restart, restart OpAMP and Agent runtime, and deliver revision 4 to a fourth sink. The extension compiled but skipped locally because pinned binaries and `AR_IMMS_PR07C_CONTROL_E2E=1` are unset. Native verification is pending a final-head CI run. |
| Mock Core credential replacement and active-session revocation | PASS / LOCAL HOST TESTS; MANAGED E2E PENDING CI | New persistent Mock Core test verifies bearer rotation survives server restart, rejects the previous token, accepts the replacement, and stores only hashes. The OpAMP session-monitor test now uses live HTTPS Mock Core and verifies an established session disconnects after bearer revocation. `go test ./internal/mockcore -count=1` and focused `go test ./internal/opampserver -run 'TestEstablishedSessionDisconnectsAfterMockCoreRevokesCredential' -count=1` passed in the authorized host environment. Managed-runtime rotation/reconnect still awaits final-head CI. |

## PR-07c final-head verification — 2026-11-13

**Verified pushed code head:** `2a7eff13251ec31c2a90be5169d90f219eb51340`
(`feat/pr-07-opamp`). The PR-07c implementation commits are `8299deb`
(installer/owner selection), `08aead6` (process containment), `b7627fb`
(managed runtime), and `2164933` (session revocation); `fffcb52` records the
prior verification status; `16b881c` makes the enabled CI gate fail rather
than silently skip on a non-elevated Windows runner; `5c85db7` corrects the
Supervisor artifact URL after run `43369304553`; `4bf9a73` splits Mock Core
persistence and the existing Collector runtime watcher into same-package files
after run `43369909210` exposed the production file-size guard. The unrelated pre-existing worktree changes and
`cmd/agentctl/otel.yaml.lifecycle.lock` remain unstaged and preserved.

| Check | Result | Evidence / limit |
|---|---|---|
| `go test ./... -count=1` | PASS / WINDOWS | Final full repository suite passed after the file splits when run outside the restricted sandbox. Windows Job Object tests ran. Protected token-file filesystem tests remain elevation-gated. A sandboxed attempt failed on denied SQLite temp-file and loopback access; the unrestricted rerun passed. |
| Focused Mock Core/OpAMP/runtime tests | PASS / WINDOWS | `go test ./internal/mockcore ./internal/opampserver ./internal/coreapi ./internal/agentlifecycle -count=1` passed after the splits. The live E2E itself was not enabled locally because the current process is not elevated and verified binaries are not provisioned for this run. |
| Architecture guard | PASS / WINDOWS | `./scripts/check-architecture.ps1` passed after same-package splits reduced `internal/mockcore/server.go` from 574 to 393 lines and `internal/agentlifecycle/runtime.go` from 456 to 155 lines. `runtime_watcher.go` is 264 lines. |
| `go vet ./...` | PASS / WINDOWS | Completed with exit code 0 after the file splits. |
| `go build ./...` | PASS / WINDOWS | Completed with exit code 0 after the file splits. |
| Linux build | PASS / CROSS-COMPILE | `GOOS=linux GOARCH=amd64 go build ./...` passed after the file splits. This is not Linux runtime or service verification. |
| Managed Supervisor live E2E | PASS / LIMITED — WINDOWS + NATIVE UBUNTU CI | Actions run [`43373790016`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43373790016) ran `TestManagedSupervisorRuntimeDeliversOTLP` and passed on Windows Server 2025 and `ubuntu-latest`. The test verifies pinned binaries, local hostmetrics fallback, Collector health, delivery of host metrics to an isolated OTLP sink, and process-tree cancellation. The preceding failures on both runners were traced to the sink and source Collectors both binding `localhost:8888`; disabling internal metrics in both test configs resolved the testbed collision. The sanitized CI summary artifact was uploaded per runner. This remains a hostmetrics smoke test, not full managed-runtime parity. |
| Native Linux / Docker | BLOCKED / LOCAL ENVIRONMENT | `wsl.exe --list --quiet` returned `Wsl/E_ACCESSDENIED`; Docker Desktop reports access denied opening its Linux Engine named pipe. Native Linux service, process-group, Docker Engine, and managed-runtime E2E remain unverified. |
| Full vertical-slice coverage | PARTIAL / GATE OPEN | Final-head run `43415093522` passed native Windows/Ubuntu enrollment, authenticated WSS policy delivery, credential replacement/revocation, offline fallback/reconnect, Core restart/publication attribution, Agent/OpAMP runtime restart, revision-5 rollback attribution, and Gateway OTLP. Ubuntu also passed isolated PR-05/06 Docker E2Es and Agent-managed Redis/PostgreSQL/Python Prometheus/mock-cooling parity with churn and duplicate-sample checks. Still open: managed local-config content update and direct unchanged-config no-op, managed injected Supervisor/Collector crashes and Agent service recovery, Agent LKG equivalence, Windows managed-profile E2E, and native install/migration/upgrade/uninstall. |
| Native vertical-slice invocation | PASS / WINDOWS + UBUNTU | Run [`43405983503`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43405983503) executed the expanded enrollment-to-WSS test on both native platforms. Local invocation still skips without pinned binaries and protected credential access; CI is the retained live evidence. |
| First final-head vertical-slice CI | FAIL / FIX IN FOLLOW-UP | Run `43392748869` executed the Ubuntu live test and exposed a missing Agent precondition: the test fixture passed a generated UUID but had not initialized it through the Agent installation-identity store, so enrollment rejected the database state before sending the request. The existing Agent identity initializer is now used in the fixture. The existing fallback smoke and Linux Docker profile E2Es passed; vet/build were skipped after the failing step. Windows job was still running when this report entry was added. The vertical slice remains NOT VERIFIED until a rerun passes on both platforms. |
| Second final-head vertical-slice CI | FAIL / FIX IN FOLLOW-UP | Run `43393916687` passed enrollment identity initialization but Collector config validation rejected a literal tab introduced by whitespace before the closing raw-string delimiter. The tab is removed in the next follow-up, and config validation errors now include the generated test YAML. Ubuntu fallback smoke and both PR-05/06 Docker E2Es passed; vet/build were skipped after the failed live step. Windows was still running when recorded. Do not count the managed vertical slice as verified until a corrected rerun passes on both platforms. |
| Corrected final-head vertical-slice CI | PASS / NATIVE WINDOWS + UBUNTU; PARTIAL ADOPTION | Run [`43405983503`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43405983503) at `e35bf05eba70d15bcc3689bcf6caee56671cd568` passed full Go tests, vet, and build on both platforms. Both passed managed enrollment, authenticated WSS, credential replacement/revocation, reconnect, revisions 1–3, and initial/updated/post-rotation Gateway OTLP. Ubuntu also passed native Docker Redis E2E and Agent-managed Docker application-profile parity. Evidence artifacts: `pr07c-managed-supervisor-Windows` (14027711358), `pr07c-managed-supervisor-Linux` (14027901228), and `pr07a-supervisor-profile-parity` (14027796168). This run predates the new Core/OpAMP/Agent restart extension. |
| Managed restart and rollback attribution | PASS / NATIVE WINDOWS + UBUNTU CI | Final-head run [`43410936502`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43410936502) passed Windows and Ubuntu full tests, vet, build, and the managed E2E. It persisted/reloaded Mock Core, verified retired-bearer rejection, resolved revision-3 publication by bound OpAMP UID/hash after restart, restarted OpAMP and Agent runtime, delivered revision 4, then reused the revision-3 hash at policy revision 5. Core attributed that hash to revision 5 and the rollback Gateway continued receiving OTLP. Evidence was downloaded under `tmp/pr07c-ci-43410936502/`: `pr07c-managed-supervisor-Windows` (14029951215), `pr07c-managed-supervisor-Linux` (14030260639), and Ubuntu `pr07a-supervisor-profile-parity` (14029448155). Earlier run [`43408766821`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43408766821) exposed the fixture's wrong installation-ID lookup; commit `31d1d68` corrected it, and run [`43409637790`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43409637790) verified restart through revision 4. |
| Managed restart and rollback attribution | PASS / NATIVE WINDOWS + UBUNTU CI | Run [`43410936502`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43410936502) passed Windows and Ubuntu full tests, vet, build, and the managed E2E. It persisted/reloaded Mock Core, verified retired-bearer rejection, resolved revision-3 publication by bound OpAMP UID/hash after restart, restarted OpAMP and Agent runtime, delivered revision 4, then reused the revision-3 hash at policy revision 5. Core attributed that hash to revision 5 and the rollback Gateway continued receiving OTLP. Evidence is retained under `tmp/pr07c-ci-43410936502/`. Earlier run [`43408766821`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43408766821) exposed the fixture's wrong installation-ID lookup; commit `31d1d68` corrected it, and run [`43409637790`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43409637790) verified restart through revision 4. |
| Agent-managed offline startup and reconnect | PASS / NATIVE WINDOWS + UBUNTU CI | Final-head run [`43412411604`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43412411604) starts Agent/Supervisor before OpAMP is listening, observes Collector readiness and fallback OTLP at the local sink, starts the authenticated OpAMP endpoint, then verifies policy revisions 1–5 and all configured Gateway deliveries. Evidence artifacts downloaded under `tmp/pr07c-ci-43412411604/`: `pr07c-managed-supervisor-Windows` (14030132774), `pr07c-managed-supervisor-Linux` (14029913823), and Ubuntu `pr07a-supervisor-profile-parity` (14030182990). |
| Local credential-rotation follow-up regression | PASS / NATIVE WINDOWS; EXPANDED MANAGED E2E SKIPPED LOCALLY | On 2026-11-13, `go test ./... -count=1`, `go vet ./...`, and `go build ./...` passed in the authorized Windows host environment. Persistent Mock Core rotation and live HTTPS active-session revocation tests passed. The expanded managed E2E compiled but skipped locally due unavailable pinned binaries and protected credential access; native evidence is run `43405983503`. |
| Expanded credential rotation and managed restart CI | PASS / NATIVE WINDOWS + UBUNTU | Run [`43405983503`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43405983503) passed protected credential rotation, old-bearer rejection, authenticated reconnect, revisions 1–3, and post-rotation OTLP. Run [`43410936502`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43410936502) passed persistent Core restart, revision-3 publication resolution, OpAMP/Agent restart, revision-4 delivery, and revision-5 prior-hash rollback attribution. Run [`43412411604`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43412411604) also passed Agent-managed offline fallback and authenticated reconnect on both platforms. |
| Agent-managed profile parity | PASS / NATIVE UBUNTU DOCKER | Run [`43405983503`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43405983503) executed `TestLiveDockerApplicationProfilesDeliverOTLP` with `AR_IMMS_PR07A_SUPERVISOR_PROFILE_PARITY=1`. The log records `owner=agent-managed profiles=redis,postgresql,prometheus,mock-cooling fallback=PASS container_churn=PASS otlp=PASS` (35.74s). It verifies no duplicate samples and no generation/config change during container churn. Windows profile E2E was skipped because CI Docker infrastructure is Linux-only. |
| Supervisor TLS CA configuration | PASS / UNIT + PINNED WINDOWS/UBUNTU E2E | Optional CA path persists in `SupervisorContext` and renders as `server.tls.ca_file`; invalid blank/control-character paths are rejected. The pinned v0.160.0 [`OpAMPServer.TLS`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/cmd/opampsupervisor/v0.160.0/cmd/opampsupervisor/supervisor/config/config.go) uses Collector `configtls.ClientConfig`. The exact binary accepted and used this setting on both platforms in run `43400294752`; TLS verification remained enabled. |
| Full Windows regression after CA-file change | PASS / WINDOWS HOST; LIVE E2E NOT RUN LOCALLY | `go test ./... -count=1`, `go vet ./...`, and `go build ./...` completed successfully in the authorized Windows environment after the CA-file implementation. A sandboxed attempt was stopped after expected SQLite and loopback permission failures. The opt-in WSS E2E still requires final-head CI because local protected credential-store elevation is unavailable. |
| Windows test/helper regression | PASS / HOST SUITE + LIVE CI | Full Windows suite, vet/build, and architecture guard passed locally. The opt-in E2E is unavailable in this local process without elevated credential-store access; native Windows CI executed and passed the managed test in 37.23s. |
| Workflow/report hygiene | PASS / PROFILE GATE ENABLED | `.github/workflows/ci.yml` sets `AR_IMMS_PR07A_SUPERVISOR_PROFILE_PARITY=1` for the Linux Redis/PostgreSQL/Python/mock-cooling E2E and uploads its evidence, so that existing Supervisor profile subcase will no longer silently skip. CI run `43397059968` predates this workflow change. Existing unrelated changes under `cmd/agentctl`, `internal/agentinstallation`, and `internal/agentlifecycle` remain unstaged and untouched by this checkpoint. |
| Draft PR and final-head CI | PASS / LIMITED; PR REMAINS DRAFT | Branch `feat/pr-07-opamp` is pushed; implementation head `2a7eff13251ec31c2a90be5169d90f219eb51340` passed run `43373790016`, and documentation-updated head `3df00f0d794b8c12e470b6dc2a9eecff6e65711f` passed run [`43374665764`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43374665764) on Windows and Ubuntu. Both ran the full Go test suite, managed Supervisor fallback-to-OTLP E2E, `go vet ./...`, and `go build ./...`; Ubuntu additionally ran the isolated Docker/Collector Redis and application-profile E2Es. PR #30 remains draft and no merge was attempted. Remaining managed-runtime gates listed above prevent PASS/merge. |

The CI gates added in this checkpoint verify the pinned Supervisor executable
digest on each runner, run the real Agent-managed hostmetrics fallback through
the pinned Collector to an isolated OTLP sink, and upload runtime evidence on
success or Collector warning/error records on failure. The Windows gate now
fails on missing elevation rather than silently skipping. CI must run on final PR head before treating that limited
smoke test as evidence. It does not replace authenticated Agent/Mock Core E2E,
credential rotation/revocation under the managed runtime, service recovery,
local configuration update/no-op checks, profile parity, or Agent LKG parity.

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

1. Managed-runtime HTTPS enrollment, protected credential provisioning and
   replacement, retired-bearer rejection, authenticated WSS reconnect, and
   offline fallback are verified on Windows and Ubuntu. `instance_uid` remains
   separate from authentication.
2. The local versioned Gateway policy and publication/status contracts are
   implemented and Mock Core-backed tests verify revision/hash attribution,
   restart persistence, and rollback. Production NestJS interoperability and
   shared-contract publication remain separate Core work; no production Core
   behavior is claimed here.
3. Map protocol/runtime evidence to truthful received, validated, applying,
   applied, failed, recovered and unknown statuses. Readiness is not scrape or
   OTLP delivery evidence; an older applied hash after rollback is not success
   for a newer desired revision.
4. Agent-managed profile parity for Redis, PostgreSQL, Python Prometheus, and a
   recreated cooling workload passed in native Ubuntu Docker CI run
   `43415093522`, including churn and duplicate-sample checks. Windows
   Agent-managed application-profile E2E remains open; Windows CI has no Docker
   profile testbed.
5. Agent process-tree cleanup uses a Linux process group and Windows Job
   Object. Managed-runtime normal shutdown and cleanup passed in the latest
   native Windows/Ubuntu E2E. Injected Supervisor crash/recovery under native
   service ownership remains unverified; an earlier raw Linux restart left an
   orphan Collector bound to OTLP and health ports.
6. Native Ubuntu Docker/Collector E2Es passed in run `43415093522`. This closes
   the earlier workstation-only Docker limitation; it does not establish
   Windows Docker parity or native service installation behavior.
7. Migration is reversible and opt-in: fresh installation retains legacy
   ownership unless an OpAMP endpoint is supplied; reinstall preserves the
   stored mode. Default transfer still requires verified managed local-config
   changes, Agent LKG equivalence, forced crash/service recovery, Windows
   profile parity, and native installation/migration/upgrade/uninstall gates.

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
| Supervisor process crash recovery | IMPLEMENTED / LIVE SERVICE GATE PENDING | Raw Linux Supervisor kill/restart failed: the orphan Collector retained ports `127.0.0.1:37035` and `127.0.0.1:8888`, so its replacement could not start. Evidence: `tmp/pr07a/evidence/linux/supervisor-crash-orphan-recovery.txt`. Agent-managed process containment now terminates the owned process tree on Supervisor exit/cancellation; Windows containment tests pass and Linux tests cross-compile. Run the managed runtime crash/restart test on native Windows/Linux and prove one sink stream before closing this gate. |
| Invalid candidate rejection and recovery | PASS / LIMITED | A syntactically valid remote config using an already-bound OTLP receiver port caused activation failure and an OpAMP `FAILED` status. The test server restored the prior config; the Supervisor reported that hash `APPLIED` and sink B delivery resumed. |
| Automatic config rollback / Agent LKG parity | PASS / LIMITED | With the desired-state test server sending only changed config hashes, the pinned Supervisor reported `FAILED` for an activation-failing config, automatically restored the previous working remote config, and emitted `APPLIED` for its hash within the 20-second observation window. OTLP resumed. The test server had to avoid replaying the same failed hash. Agent Runtime LKG parity remains NOT RUN. |
| Actual OTLP delivery | PASS / LIMITED | Windows and Linux hostmetrics reached isolated sink A during fallback and sink B after remote update, activation recovery, Collector crash restart, and coordinated Supervisor process restart. Redis, PostgreSQL, Python Prometheus, and mock-cooling profiles reached the isolated sink under Windows Supervisor fallback and OpAMP reconnect. |
| Existing application-profile parity | PASS / LIMITED | Windows Docker E2E and Ubuntu WSL/Docker Desktop E2E reused PR-06 Redis, PostgreSQL, Python Prometheus, and mock-cooling profiles. Agent Runtime emitted all four streams; after stopping it, Supervisor emitted the same profiles from startup fallback and after OpAMP reconnection. Container recreation delivered the new cooling `resource_id`; tests found no Agent-created duplicate receivers or duplicate OTLP samples. Native Linux Docker Engine parity remains BLOCKED; hostmetrics was covered by the separate Supervisor/Collector E2E. |
| Windows native E2E | PASS / LIMITED | Focused Windows live test with exact binaries, local `opamp-go` server, and two isolated Collector OTLP sinks. This is a smoke test, not complete PR-05/06 parity. OS build number was not captured. |
| Linux-in-WSL Supervisor/Collector/OTLP E2E | PASS / LIMITED | Ubuntu 24.04 WSL2 kernel `6.18.58.1-microsoft-standard-WSL2`, pinned Linux Supervisor and Collector, local OpAMP test server, and isolated OTLP sinks. This is not native Linux service or Docker Engine verification. |
| Linux Docker profile parity | PASS / WSL + DOCKER DESKTOP | Ubuntu WSL Go Agent and pinned Linux Supervisor/Collector exercised Redis, PostgreSQL, Python Prometheus, mock cooling, container recreation, profile identity, no duplicate samples, startup fallback, reconnect, and OTLP delivery against Docker Engine 29.5.3. `docker info` identifies `Docker Desktop (containerized)`; this does not verify a native Linux Docker Engine. Evidence: `tmp/pr07a/evidence/linux/application-profiles/`. |
| Native Linux Docker Engine parity | BLOCKED | No native Linux Docker Engine is available in this environment; the available daemon is Docker Desktop. Run the same test on a native Linux Docker host before treating platform parity as complete. |
| Mock Core contract interoperability | PR-07B | See the PR-07b local contract and protocol verification section below. |
| Production NestJS Core interoperability | BLOCKED | Core control operations are absent from inspected API baseline; requires reviewed contract and a separate Core implementation. |
| Reversible lifecycle ownership selection | IMPLEMENTED / FOCUSED TESTS | Explicit install opt-in records Supervisor ownership; reinstall preserves it; legacy installs continue using the existing Agent runtime. Live upgrade/uninstall and rollback to prior owner remain unverified. |

## PR-07b local contract and protocol verification

| Gate | Status | Evidence / note |
|---|---|---|
| Local versioned Core control contract | PASS / MOCK-BACKED | `docs/local/architecture/v0.1.4a-next/contracts/agent-control-v1.openapi.yaml` defines authenticated session/policy read, first-bind compare-and-set, durable publication record/lookup, and truthful observed status. It is not published to `docs/shared/` and is not a production Core contract. |
| Existing enrollment bearer and Core response conventions | PASS / MOCK-BACKED | TLS `httptest` integration exercises HTTPS-only client, existing bearer, `{data,meta.request_id}` responses, RFC problem responses, revocation, binding conflict, and identity mismatch. Duplicate identical status PUTs are idempotent in Mock Core. |
| OpAMP server policy and status translation | PASS / MOCK-BACKED | Live `opamp-go` v0.23.0 WebSocket client connects to the Go adapter and HTTPS Mock Core, receives the versioned Gateway policy, binds its instance UID, and reports APPLIED revision 1. See `go test ./internal/opampserver -run '^TestLiveAuthenticatedSupervisorToMockCorePolicyAndStatus$' -count=1 -v`. |
| Pinned Supervisor bearer-auth config | PASS / WINDOWS + UBUNTU WSL | Exact Supervisor/Collector v0.160.0 with `server.auth: bearertokenauth` and the `opampsupervisor.Extensions` gate connected to the adapter; Mock Core recorded the bound instance and APPLIED revision 1 on both operating systems. The isolated test disables Collector internal metrics to avoid colliding with an existing `localhost:8888` listener and sets a 15s Collector bootstrap timeout. Evidence: `tmp/pr07b/evidence/{windows,linux}/status-summary.txt` and `supervisor.log`. |
| Remote config status attribution safeguards | PASS / MOCK CORE INTEGRATION | Unknown hashes are suppressed. The process-local map remains a bounded duplicate-delivery optimization; correctness resolves publications through the authenticated Core contract. Tests verify an old hash after adapter restart reports `desired_revision=2`, `publication_revision=1`, and `applied_revision=1`, without claiming revision 2 was applied. Existing pinned Windows/WSL live revision-attribution evidence remains recorded above. |
| PR-07b Go package regression | PASS / WINDOWS | `go test ./internal/coreapi ./internal/opampserver ./internal/mockcore ./cmd/agentctl -count=1` passed through the authorized Windows environment. Sandbox-local TLS loopback execution is denied by OS socket permissions; that failed invocation is environmental, and the same suites passed in the authorized environment. |
| Repository Go regression, vet, and builds | PASS / WINDOWS + UBUNTU WSL | `go test ./... -count=1`, `go vet ./...`, and `go build ./...` passed on Windows and Ubuntu 24.04 WSL with Go 1.25.5. WSL run uses `GOMODCACHE=/tmp/codex-pr07-gomodcache`, `GOCACHE=/tmp/codex-pr07-gocache`; this verification also confirmed test-only PR-07a helper files compile on both operating systems. |
| Production NestJS Core interoperability | BLOCKED | Inspected Core API Baseline contains no Agent control routes. Local Mock Core is replaceable by endpoint configuration but does not prove production interoperability. |
| Supervisor credential provisioning by Agent | PASS / WINDOWS + NATIVE UBUNTU MANAGED E2E | Agent runtime resolves the existing enrollment credential reference, materializes the protected token file, and restarts the contained Supervisor when the reference changes. Final-head managed E2E verifies actual enrollment, credential replacement and retired-bearer rejection, authenticated WSS reconnect, remote policy, Core restart, and OTLP delivery. Service install/migration/uninstall remains open. |
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

## Latest final-head verification follow-up

- Run [`43413389730`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43413389730)
  executed the Windows full Go test step and failed only at
  `TestPeriodicSQLitePollingDetectsCommittedCrossProcessDesiredChange`; the
  failure was `startup reconciliation did not run` after a transient SQLite
  snapshot warning. Vet/build and opt-in live E2Es were skipped after that
  failure; Ubuntu was still running when inspected. The test's one-second
  startup deadline was too brittle for a Windows runner under load. Its
  deadline is now three seconds and its cancellation is deferred so a failed
  assertion cannot leave the poller running. On the authorized native Windows
  host, the focused test passed 100 consecutive runs. This is a test-harness
  adjustment; no SQLite or reconciliation production behavior changed. Exact
  head `78981ea60db3b2950e38e579e89be460da4c8fb1` subsequently passed both
  native jobs in run `43415093522`.
- The latest PR head was `bed1e624c0ee416e0559992b77bef2c1d6fc318b` when run
  `43413389730` was inspected. Commit `78981ea` contains the test and report
  follow-up and is pushed to the existing feature branch.

## Final-head verification after SQLite polling adjustment

- Run [`43415093522`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43415093522)
  passed on both native Windows and Ubuntu at PR head
  `78981ea60db3b2950e38e579e89be460da4c8fb1`. Both jobs executed the full Go
  test step, Agent-managed Supervisor enrollment-to-OTLP E2E, `go vet ./...`,
  and `go build ./...`; the architecture guard passed on both. Ubuntu also
  executed and passed isolated PR-05 recovery and PR-06 application-profile
  Docker E2Es, followed by Agent-managed profile parity.
- The managed E2E evidence records actual enrollment, protected credential
  replacement and retired-bearer rejection, offline fallback/reconnect,
  authenticated WSS, policy revisions 1–5, durable Mock Core restart and
  publication lookup, OpAMP/Agent runtime restart, revision-5 rollback hash
  attribution, and OTLP delivery. The Ubuntu profile artifact reports
  `owner=Agent-managed Supervisor v0.160.0`, profiles Redis/PostgreSQL/Python
  Prometheus/mock-cooling, container churn, and post-churn OTLP.
- Evidence was downloaded under ignored `tmp/pr07c-ci-43415093522/`:
  Windows managed runtime artifact `14032066006` (SHA-256
  `0c704f21de5f99a403e0ce6521787ad87adb39040ddbdab0fd0abb5cc1a45212`), Linux
  managed runtime artifact `14030809627` (SHA-256
  `18ca53464bf7515047d145d0c396c8777689398be085c21c593517cafb43a587`), and
  Ubuntu profile parity artifact `14031950330` (SHA-256
  `60ac3e8118145cc1ed0b63765912b57c4199412ef5ab94c0194e0393604157fc`). Both
  managed artifacts include sanitized pass summaries; hashes and policy
  revisions match the workflow artifact metadata.
- Authorized native Windows checks also passed `go test ./... -count=1`,
  `go vet ./...`, `go build ./...`, and `./scripts/check-architecture.ps1`;
  the adjusted polling regression passed 100 consecutive runs. The local
  managed binary E2E was not run in this process; CI executed it natively.
- This run supersedes the failed documentation-only run `43413389730` for the
  current PR head. PR #30 remains draft and PR-07 remains IN_PROGRESS. The
  report does not treat CI's skipped Windows Docker tests as Windows profile
  verification.

## Managed process crash gate added for exact-head CI

- A test-only extension to `TestManagedSupervisorEnrollmentAndRemoteGatewayE2E`
  now kills the Agent-managed Collector by matching its pinned executable and
  exact managed config path, then requires one replacement process and renewed
  OTLP delivery. Sink Collectors use different config paths and are excluded.
- The same test kills the Agent-managed Supervisor, requires the Agent runtime
  to report the unexpected exit, waits for both Supervisor and its Collector
  process to disappear, and checks that the isolated sink stops receiving data.
  It then restarts the Agent runtime and requires readiness plus renewed OTLP.
- Local verification: `go test ./internal/agentlifecycle -run
  '^TestManagedSupervisorEnrollmentAndRemoteGatewayE2E$' -count=1` passed with
  the live E2E skipped because `AR_IMMS_PR07C_CONTROL_E2E` and the protected
  credential-store runtime prerequisites are unavailable in this process.
  The full `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and
  `./scripts/check-architecture.ps1` passed in the authorized native Windows
  environment after the test change. A sandboxed full-suite attempt failed on
  denied SQLite temp files and loopback sockets; this is not the native run's
  result. The opt-in live test was attempted in the current process but this
  Windows token is not elevated, so protected credential provisioning stopped
  before runtime startup.
- The workflow explicitly sets `AR_IMMS_PR07C_CONTROL_E2E=1` and runs this test
  on native Windows and Ubuntu. Crash recovery and process-tree evidence remain
  NOT VERIFIED until that exact-head workflow completes successfully. PR #30
  remains draft; Supervisor remains opt-in.
- Exact-head run [`43419406085`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43419406085)
  executed the crash-enabled E2E on both operating systems but timed out while
  looking for the managed Collector. The runtime launches a snapshot under a
  `.collector-launch-*` directory, so matching the original config file path
  was incorrect. The fix moves all four sink Collector configs to independent
  temp directories and matches the unique Agent runtime temp root, which is
  present in its snapshot path. Both jobs passed full tests; Ubuntu also passed
  the PR-05 Docker recovery and PR-06 application profile steps. The crash gate
  remains open until a new exact-head run passes.

## Final managed crash-injection verification

- Exact PR head `6429d4bef925d6cabd5b14ea25565df0c5f6f517` passed CI run
  [`43420887371`](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43420887371)
  on native Windows and Ubuntu. `TestManagedSupervisorEnrollmentAndRemoteGatewayE2E`
  executed and passed in 70.53s on Windows and 66.98s on Ubuntu; both full Go
  test steps, `go vet ./...`, and `go build ./...` passed. Ubuntu also executed
  isolated Docker recovery, application profiles, and profile parity; these
  steps were skipped on Windows because the hosted Windows runner has no Docker.
- Failure injection terminated the Agent-managed Collector, observed exactly
  one replacement Collector, and verified OTLP resumed. Terminating the
  Agent-managed Supervisor caused the Agent runtime to return an error, cleaned
  up both managed processes, and stopped sink delivery. Restarting the Agent
  runtime restored readiness and OTLP delivery. Evidence summaries explicitly
  report `collector_crash_recovery=verified_single_replacement_and_otlp`,
  `supervisor_crash_process_tree_cleanup=verified`, and
  `agent_runtime_restart_after_supervisor_crash=verified`.
- Sanitized evidence is downloaded under
  `tmp/pr07c-ci-43420887371/`. Windows managed-runtime artifact `14033099323`
  has GitHub SHA-256 `6a03268a756fbdc74734386c9031d82b314bdc7e83112373bb08a0d355cda3bf`;
  Ubuntu artifact `14034381146` has SHA-256
  `7e4f2a74072b2b04b12d6ee4ef7dfbce2cca6149218809c1aace0f0912c97f97`.
  Ubuntu profile parity artifact `14032864851` has SHA-256
  `cf87dae6147ab0df4e53b35bd06391a58f1717bfab884a2e7de213437f3885c3`.
- This closes the managed Collector/Supervisor crash and Agent runtime restart
  gate for the E2E harness. Remaining mandatory adoption gates: managed local
  configuration content update and direct unchanged-config no-op, Agent LKG
  equivalence, Windows Agent-managed profile E2E, and native Windows/Linux
  installation, migration, upgrade, and uninstall. The CI process-recovery test
  does not establish OS service migration lifecycle. PR #30 remains draft and
  PR-07 remains IN_PROGRESS; Supervisor remains opt-in.
