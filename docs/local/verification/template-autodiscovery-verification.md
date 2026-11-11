# Template-Driven Automatic Monitoring Verification

**Checkout:** `feat/template-driven-auto-monitoring`, working tree at verification time. This report does not claim changes are committed, merged, or deployed.

## Verification performed

| Check | Result | Evidence |
|---|---|---|
| Template file workflow and export | **PASS — Windows amd64** | Focused CLI tests cover file create/update, full `inspect --output json`, legacy `--template-json`, strict invalid JSON/unknown-field rejection, validation errors, and stale revision conflict. A separate `go run ./cmd/agentctl` transcript created the supplied JSON file in isolated temporary SQLite state, exported the full stored Template, updated `15s` to `30s`, then disabled, enabled and tombstoned it through public CLI commands. |
| Public Integration catalog | **PASS — Windows amd64** | `agentctl integration catalog list` reports the registered `redis@1.0.0` Definition, Template settings with type/default/constraints, and capability status from the Windows amd64 pinned artifact manifest. `catalog inspect --output json` tests cover selector, setting and native pipeline mapping; unknown `grafana@1.0.0` returns an actionable unregistered-Definition message. |
| Integration, Configuration, SQLite state, Discovery, CLI package tests | **PASS** | `go test ./internal/integration/... ./internal/configuration ./internal/agentstate ./internal/discovery/... ./cmd/agentctl -count=1` — all listed packages passed outside the Windows sandbox. |
| Repository tests | **PASS — Windows amd64** | `go test ./...` — all packages passed outside the Windows sandbox after the Template/catalog changes. |
| Race tests | **NOT RUN for this follow-up** | The earlier S4 verification record covered race testing before the Template file/catalog follow-up; it is not claimed as verification for this follow-up. |
| Static analysis | **PASS** | `go vet ./...` — exit status 0 outside the Windows sandbox. |
| Build | **PASS** | `go build -buildvcs=false ./...` — exit status 0 outside the Windows sandbox. |
| Runtime Template lifecycle test | **PASS** | `TestRuntimeTemplateMaterializerPreservesFailureAndTracksResourceLifecycle`: successful projection, provider timeout, first complete miss, confirmed second miss, and reappearance with the same stable instance ID. |
| Runtime multi-resource and no-op test | **PASS** | `TestRuntimeTemplateMaterializerProjectsStableInstancesAndNoOps`: two resources become distinct generated instances; repeated refresh does not advance desired or instance revisions; disabling the Template suspends generated instances. |
| Live Docker Template test | **PASS** | `TestLiveDockerTemplateAutoMaterializesDesiredInstance` ran against one uniquely named disposable `redis:7-alpine` container, an ephemeral localhost-published port, the local Docker named pipe, and isolated temporary SQLite state. It confirmed automatic generated desired state and no duplicate manual approval proposal. |
| Template → Agent runtime → Collector → OTLP sink | **PASS — Windows amd64** | `TestLiveDockerTemplateConvergesThroughAgentRuntime` ran against three disposable Redis 7 containers with isolated SQLite/config paths, the checksum-verified Collector 0.160.0 binary, and a local OTLP/debug sink. It observed three enabled Template-owned desired instances, M2 application, per-endpoint Redis metrics, Template update to all three, disable/re-enable/delete activation, unchanged-projection stability through a full discovery/reconciliation interval, and restart of the Agent runtime with persisted enabled instances and renewed telemetry. Hostmetrics and `otlp/gateway` remained present throughout. This exercises the runtime composition used by the Agent Service; it does not install/start the Windows Service Control Manager service. |
| Docker Compose restart/recreation lifecycle | **PASS — Windows amd64** | `TestLiveComposeRedisLifecycleConvergesWithoutGhostInstances` ran two uniquely named Compose projects with the same `redis` service name; the first project also ran PostgreSQL and Python discovery fixtures. It created both Templates through the public `--file` CLI and changed project A's collection interval through a revision-checked file update while S3/M2 was running. Ordinary restart preserved container identity and caused no desired revision or Collector generation change with a fixed published port. Compose down/up created a new Redis ID. The first successful absence snapshot intentionally retained both old and new generated instances, and the test observed their Redis receivers in the applied configuration; after the second successful absence, the old instance was disabled and the effective config contained only the new project-A and surviving project-B receivers. `--force-recreate` was independently exercised for project B; its old/new overlap was observed and removed after the second miss. Active inventory excluded both old IDs while history retained them as missing. Host metrics and Gateway config remained present, and Redis telemetry was observed from current endpoints in the isolated OTLP sink. |
| Three-instance telemetry attribution | **PASS — Windows amd64** | Three disposable Redis containers were discovered and materialized from one Template. `TestLiveDockerTemplateConvergesThroughAgentRuntime` required each receiver endpoint's `server.port` resource attribute and `redis.clients.connected` in the same OTLP sink resource block. The Redis contributor now enables those receiver-provided resource attributes, which are disabled by default in the pinned receiver. |
| Exact pinned Collector manifest and Redis candidate validation | **PASS — Windows amd64** | Downloaded the official 0.160.0 Windows amd64 release archive to a temporary directory, verified SHA-256 `d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf` against `internal/bootstrap/artifact.go`, then ran `TestRedisCandidateNativeValidationWhenPinnedBinaryProvided` and `TestStaticManifestMatchesActualCollectorBinaryWhenProvided`; both passed. |
| Exact pinned Collector manifest and Redis candidate validation | **PASS — Linux amd64** | In a Linux Go container, downloaded the official 0.160.0 Linux amd64 release archive, verified SHA-256 `7bb60c584c241c86261c2b8697cd3725dd8c56691f5ad5d98454eaa005b47b0c` against `internal/bootstrap/artifact.go`, then ran `TestNativeValidationWhenPinnedBinaryProvided`, `TestRedisCandidateNativeValidationWhenPinnedBinaryProvided`, and `TestStaticManifestMatchesActualCollectorBinaryWhenProvided`; all passed. A temporary `/etc/machine-id` fixture was mounted only in the test container for platform-info collection. |
| Linux live Docker runtime E2E | **BLOCKED — Docker Desktop host networking** | The test ran the Linux Collector and Agent in a disposable Linux container with the Docker socket. Discovery produced the host-published loopback endpoint `127.0.0.1:<port>`, but it was not reachable from the isolated test container's network namespace; Redis scrapes returned `connection refused` until timeout. The exact disposable Redis fixture, runner and temporary module-cache volume were removed. A native Linux host/runner is required for this end-to-end network topology. |

The SQLite tests failed inside the Windows sandbox with `unable to open database file (14)`. Rerunning the same focused/affected package suites outside the sandbox against their isolated test databases passed. No production database was used.

## Acceptance criteria

| ID | Status | Evidence / remaining scope |
|---|---|---|
| AC-01 | **PASS — Windows runtime E2E** | Live Docker discovery → Template-owned desired Redis instance → S3/M2 application → pinned Collector → isolated OTLP sink metric passed. |
| AC-02 | **PASS** | Runtime test creates two stable, distinct IDs from one Template; domain test covers logical identity excluding Template revision. |
| AC-03 | **PASS — Windows runtime E2E** | The live three-container test repeated unchanged discovery/materialization and observed no desired revision, M2 `AppliedGeneration`, or effective-config change through a full discovery/reconciliation interval. This verifies no extra lifecycle application; it does not inspect the Collector process ID directly. |
| AC-04 | **PASS** | Runtime SQLite test changes the Template interval, verifies both owned generated instances update, and verifies a manual Integration retains its original revision and settings. |
| AC-05 | **PASS — Windows runtime E2E** | Disabling, re-enabling, and deleting the Template changed only its generated instances and activated configs without Redis receivers; hostmetrics and Gateway components remained. Deleted Template records stayed disabled. |
| AC-06 | **PASS** | Configuration/SQLite ownership guards and materialization conflict tests prevent Template projection from mutating manual or foreign-owned instances. |
| AC-07 | **PASS** | Matcher/domain tests reject secret-bearing values, unsupported/unverified capability, unavailable binding inputs and invalid settings. |
| AC-08 | **PASS** | Matcher evaluates the full Template set and marks conflicting Template/resource/Integration pairs non-materializable independent of order. |
| AC-09 | **PASS** | Runtime lifecycle test simulates provider timeout and proves desired instances/revision remain unchanged; runtime loop test skips projection after provider failure. |
| AC-10 | **PASS** | Runtime test proves one complete miss preserves the instance, second complete miss suspends it, same-resource reappearance restores its ID, and a new resource ID creates a distinct instance. |
| AC-11 | **PASS — Windows runtime E2E** | The test stopped and restarted the Agent runtime composition with the same SQLite state while three generated instances were enabled, then verified the Collector candidate contained them and Redis telemetry resumed. It does not install/restart the Windows Service Control Manager service. |
| AC-12 | **PASS** | Existing proposal/approval tests remain green in the full affected CLI and Configuration package suites. |
| AC-13 | **PASS — Windows runtime E2E** | The generated instance was persisted by Configuration and applied through the existing S3/M2 runtime; no CLI reconciliation or alternate activation path was used. |
| AC-14 | **PASS** | Template, desired-set, per-instance and M2 generation/revision values remain separate in their models and persistence; persistence and reconciliation tests pass. |
| AC-15 | **PASS** | Generated instance validation and runtime tests cover Template owner/ID/revision and provider-scoped resource binding. |
| AC-16 | **PASS** | Template service accepts local ownership only; CLI rejects Core ownership. No Core synchronization behavior was added. |
| AC-17 | **PASS** | Template, Integration, and contribution validation reject secret-bearing values; focused unit suites pass. |
| AC-18 | **PASS — Windows Compose E2E** | Real `docker compose restart`, `down`/`up`, and `up --force-recreate` runs prove stable ID for restart, distinct IDs for recreation, isolated same-name services across projects, the measured two-snapshot overlap, removal from the final effective config, active-vs-history inventory, and current endpoint telemetry delivery. |
| AC-19 | **PASS — Windows amd64** | Public CLI tests and a temporary-state `go run` transcript create and update a Template from a JSON file, export the full stored object, preserve legacy inline JSON input, reject stale revisions without overwriting the stored row, and exercise enable/disable/delete. The live Compose E2E applies a file-based Template update through S3/M2. |
| AC-20 | **PASS — Windows amd64** | `integration catalog list` and exact-version `inspect` expose the static Redis Definition, setting metadata and artifact-bound capability mapping; unsupported Grafana is reported as unregistered. The guide documents the actual static extension workflow and explicitly states Grafana remains unimplemented. |

## Group A — Automatic Redis monitoring

| Scenario | Status | Notes |
|---|---|---|
| Test 1: one Template, automatic monitoring and telemetry | **PASS — Windows amd64** | The opt-in runtime E2E proved generated desired state, applied generation, preserved host/Gateway config and Redis metrics at the isolated OTLP sink. |
| Test 2: one Template, three Redis containers, distinct telemetry per instance | **PASS — Windows amd64** | Live Docker discovery generated three stable desired instances. The pinned Collector's Redis receiver emitted its configured `server.port` resource attribute; the test verified each distinct published port alongside `redis.clients.connected` in the isolated OTLP sink. |
| Test 3: Template update changes all matching candidates while preserving host pipeline | **PASS — Windows amd64** | The three-container live test changed `collection_interval` to `15s`; all three generated instances were recompiled/applied, then hostmetrics and `otlp/gateway` were verified in the effective config. |

## Group B — Lifecycle and failure scenarios

| Scenario | Status | Notes |
|---|---|---|
| Template disable/re-enable and generated-instance suspension | **PASS — Windows runtime E2E** | Disabling and re-enabling applied Collector configurations with all three generated instances suspended/restored while baseline pipelines stayed intact. |
| Template delete and permanent tombstone | **PASS — Windows runtime E2E** | Deleting the Template applied a config without its generated Redis receivers; generated rows remained disabled under the retained tombstone. |
| Resource stop/removal/recreation | **PASS (simulated inventory lifecycle)** | Complete miss threshold, same-ID reappearance, and new-ID recreation tested; live Docker discovery is separately verified. |
| Docker/provider outage | **PASS (simulated provider timeout)** | No destructive projection on failure. |
| Agent restart | **PASS — Windows runtime E2E** | The runtime composition restarted from the same persisted DB while the Template and three generated instances were enabled; Collector config and Redis telemetry resumed. SCM installation/restart was not exercised. |
| Invalid Template, conflicting Templates, unsupported capability | **PASS (focused domain/matcher tests)** | Fail-closed results covered. |

## Group C — Heterogeneous infrastructure

**NOT RUN for Template-driven activation.** Existing matcher fixtures for PostgreSQL/MongoDB-like resources are not production Integration definitions or evidence of supported Collector pipelines. Redis remains the only production Integration in this implementation path.

## Remaining verification gates

1. Linux artifact-manifest probing and native candidate validation passed. Run the live Linux telemetry E2E on a native Linux host/runner; Docker Desktop's isolated host-loopback mapping prevents this container-based setup from reaching the discovered Redis endpoint.
2. Add runtime fault injection for a Redis-specific activation/readiness failure if the product requires evidence beyond the existing M2 recovery tests. The existing generic activation/recovery tests passed in `go test ./...`.
3. Group C remains open: no production PostgreSQL/MongoDB Integration definitions or corresponding verified Collector capabilities are included in this feature.

## Reproducing the live Template runtime E2E

Provide a disposable running Redis container with a host-published TCP port and the local Docker API endpoint. Point `OTELCOL_CONTRIB_BINARY` at the verified 0.160.0 binary, then run:

```powershell
$env:AR_IMMS_S4_TEMPLATE_RUNTIME_E2E = '1'
$env:AR_IMMS_S4_TEMPLATE_E2E_CONTAINER_IDS = '<redis-container-id-1>,<redis-container-id-2>,<redis-container-id-3>'
$env:AR_IMMS_DOCKER_DISCOVERY_ENDPOINT = 'npipe:////./pipe/docker_engine'
$env:OTELCOL_CONTRIB_BINARY = '<verified-otelcol-contrib.exe-path>'
go test ./cmd/agentctl -run '^TestLiveDockerTemplateConvergesThroughAgentRuntime$' -count=1 -v
```

The test creates state, config, and OTLP sink files under `t.TempDir()` and stops its Collector processes during cleanup. Provide only disposable Redis containers; remove only the fixtures created for the run.
