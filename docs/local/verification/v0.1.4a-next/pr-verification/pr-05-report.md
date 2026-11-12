# PR-05 verification report — OTel-native Docker/Redis discovery

**Status:** IN_PROGRESS — Windows and Linux Docker-to-OTLP live delivery and profile-composed LKG validation/activation rollback verified; daemon recovery, receiver uniqueness, and live unreachable diagnostics remain.
**Release target:** `v0.1.4a-next`
**Review date:** 2026-11-12
**Worktree:** `feat/v0.1.4a-next-work`, base worktree commit `59c85eed375c77653bcfcc57022ee01093467bc6`
**Implementation commit:** `0f03c25` (`feat(discovery): add OTel-native Docker Redis monitoring`)
**Publication:** Draft [PR #28](https://github.com/AR-IMMS/telemetry-agent/pull/28) targets `chore/v0.1.4a-next`. This report does not claim PASS.
**Current verified code/test head:** `300d6b1`; GitHub Actions run [43238401049](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43238401049) passed on Windows and Ubuntu, including Linux live Docker/Collector/OTLP E2E.
**Linux live E2E:** GitHub Actions run [43238401049](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43238401049) passed on `ubuntu-latest`, including the real Docker/Collector/OTLP profile E2E.

## Accepted contract implemented

This checkpoint implements the user-approved PR-05 contract in [discovery-monitoring-contracts.md](../../../architecture/v0.1.4a-next/discovery-monitoring-contracts.md): `node_id` comes from the registered Core binding; Docker container ID is `resource_id`; `workload_id` is optional; container name is display metadata. Docker/Redis monitoring is opt-in through the three approved labels. Agent inventory and policy remain Agent-owned, while OTel Observer and Receiver Creator own the dynamic Redis receiver for matching targets. Unlabelled/manual targets keep the existing Agent receiver path. Inventory churn does not change the native profile's rendered configuration or activate a new generation.

The native layer is enabled only when a Docker endpoint, registered Node ID, and pinned Redis capability are available. It reuses existing Collector candidate validation, activation, readiness, and LKG recovery paths; no database migration or shared-submodule edit was made. Provider label mapping is allowlisted. Incomplete/provider-error inventory snapshots retain prior resources.

## Windows evidence

Environment: Windows/amd64, Go `go1.25.5 windows/amd64`, Docker Desktop Engine `29.5.3` (`linux/x86_64`), Collector `otelcol-contrib 0.160.0` (SHA-256 `43A7D3EB2B759281B6297A91C830988BE39C16E538E8F55AEB4EA31AB7A612CD`). The live test used disposable Redis 7 containers, the real local Docker Engine, the Agent runtime, the pinned Collector, and an isolated OTLP receiver/sink.

| Evidence level | Result | Evidence |
|---|---|---|
| Discovered | PASS | The Agent inventory contained two opted-in published containers and one opted-in container without a published host port, all with distinct container IDs and present state. |
| Receiver active | PASS (observed) | Final pinned Collector logs contained 5 Receiver Creator start records, 1 stop, and no scrape errors across mapped targets and recreation. The log sequence repeated start records for the same endpoint ID before a stop record; this does not prove active receiver uniqueness. |
| Collection verified | PASS | `redis.clients.connected` points were observed for both initial instances and for the recreated instance. |
| Delivery verified | PASS | The isolated OTLP sink received points carrying the expected Core `node_id`, distinct `resource_id` values, shared `workload_id=cache-service`, and display container name. |
| Single owner / churn | PASS for Agent-vs-OTel ownership; BLOCKED for receiver uniqueness | Agent desired instances remained empty and no static Agent Redis receiver was generated. Removal/recreation left applied generation and config bytes unchanged. Multi-network logs repeated starts for one endpoint ID before a stop; one active receiver and duplicate-point absence are not established. |
| Endpoint cases | PARTIAL | The final run used nondefault published host port `59980` while the Redis container listened on private port `6379`, attached one container to two Docker networks, and retained an unpublished target in inventory. The Windows inventory diagnostic classified the unpublished target as missing a supported endpoint. The profile now constrains Redis rules to private port `6379`; a nondefault private Redis port is unsupported and was not tested. No live ambiguous or unreachable endpoint was induced. |

Executed live command:

```powershell
$env:AR_IMMS_PR05_NATIVE_RUNTIME_E2E = '1'
$env:AR_IMMS_PR05_COLLECTOR_BINARY = '<checksum-verified otelcol-contrib 0.160.0 path>'
go test ./cmd/agentctl -run '^TestLiveNativeDockerRedisProfileDeliversOTLP$' -count=1 -v
```

Result: PASS (14.83 seconds). The test removes its disposable containers and network. Final scenario facts are retained in [PR-05 Windows live evidence](./pr-05-windows-live-evidence.md). The separate pinned-binary profile configuration validation also passed.

## Native Linux evidence

The workflow runs the same live test on GitHub's `ubuntu-latest` Linux runner after downloading and checksum-verifying the pinned Linux Collector archive. Run [43238401049](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43238401049) passed the Ubuntu test, live E2E, vet, and build steps. The live test passed in 17.04 seconds with a Docker-assigned host port of `32770`, a target attached to two Docker networks, an unpublished target, distinct resource identities, container recreation, and isolated OTLP delivery. The full run logs include Receiver Creator start/stop events and no scrape errors. See [PR-05 Linux live evidence](./pr-05-linux-live-evidence.md).

## Build and test gates

On the same Windows worktree, all passed:

```text
go test ./... -count=1
go vet ./...
go build ./...
GOOS=linux GOARCH=amd64 go build ./...
```

The local Windows `GOOS=linux` build is a cross-build only. Ubuntu WSL2 is installed but has no Go toolchain; the native Linux E2E was therefore run on the GitHub-hosted Ubuntu Linux runner instead.

After the endpoint diagnostics, CLI inspection, provider recovery test, and profile-composed failure injections were added, the full Windows suite, vet, build, and Linux/amd64 cross-build were rerun and passed. The full suite run set `AR_IMMS_PR05_COLLECTOR_BINARY` to the checksum-verified v0.160.0 Windows binary, enabling both profile failure tests.

GitHub CI run `43238401049` on head `300d6b1` passed on `windows-latest` and `ubuntu-latest`; both jobs completed test, vet, and build. Ubuntu additionally ran the live Docker/Collector/OTLP E2E.

## Profile candidate failure and LKG evidence

With the checksum-verified Windows Collector v0.160.0 binary, both profile-composed failure injections passed:

```powershell
$env:AR_IMMS_PR05_COLLECTOR_BINARY = '<checksum-verified otelcol-contrib 0.160.0 path>'
go test ./internal/agentlifecycle -run '^TestPinned(NativeProfileActivationFailureRollsBackToLKG|InvalidNativeProfileCandidateRestoresPreviousConfiguration)$' -count=1 -v
```

The invalid Receiver Creator observer configuration is rejected at final validation, and the previous active config/state remain intact. A valid native profile using an unavailable Docker endpoint passes candidate validation, fails Collector runtime activation/readiness, and restores the previous configuration/state. Both tests assert no native `receiver_creator` remains in the restored config. This is Windows process-level lifecycle evidence, not native Linux or Docker daemon recovery evidence.

## Remaining verification gates

These items remain open and prevent PASS:

- Actual Docker daemon outage followed by recovery and convergence of Agent inventory and OTel Observer. A new coordinator unit test covers provider failure followed by recovery and endpoint refresh, but it does not stop or isolate Docker Desktop and cannot establish receiver recovery.
- End-to-end diagnostics for every state. `agentctl inventory inspect` reports `endpoint_missing`, `endpoint_ambiguous`, and `endpoint_resolved`; resolved means reachability is unverified. A parser unit test accepts the pinned Receiver Creator dynamic error record and maps it to unreachable evidence. No live scrape failure was induced, and runtime Collector output is not yet routed into the Agent diagnostic surface.
- Nondefault private Redis port behavior. PR-05's initial Redis profile explicitly supports private port `6379`; a nondefault published host port is verified, but alternate private Redis ports are unsupported and were not tested.
- Multi-network receiver uniqueness and duplicate point behavior. The live run verifies two-network discovery and delivery, but Collector logs contain repeated starts for the same endpoint ID before a stop. Whether this produced overlapping receiver instances or duplicate points remains unresolved; do not mark single-receiver behavior PASS from these logs.
- The unpublished Windows target was discovered and diagnosed missing; a live no-receiver assertion is not retained.
- Windows/Linux service-level behavior beyond the process-level live test.

Coordinator tests verify that failed observations preserve present inventory and a recovered snapshot clears the provider error and refreshes endpoint metadata. These use a fake provider and are unit evidence, not live Docker daemon recovery or Receiver Creator convergence. Do not infer the remaining behaviors from the successful Windows path.

## Changed scope and preservation

Agent Go source and tests, local architecture/plan/progress documentation, and this verification report are the intended PR-05 changes. No production configuration, schema, database migration, `docs/shared` content, or runtime artifact was added. The pre-existing untracked `cmd/agentctl/otel.yaml.lifecycle.lock` is preserved and is not part of PR-05. Temporary Collector/test artifacts are removed after verification.

**Checkpoint ruling:** Windows and native Ubuntu runner coverage includes multi-network observations, nondefault published host ports, an unpublished target, inventory diagnosis, profile extensibility, profile-composed LKG recovery after validation and activation failures, and OTLP delivery. PR-05 remains **IN_PROGRESS / CONDITIONAL** because actual daemon recovery, live unreachable reporting, Windows multi-network receiver uniqueness/duplicate points, and nondefault private Redis ports remain open. PR-06 remains blocked by PR-05.
