# PR-05 verification report — OTel-native Docker/Redis discovery

**Status:** IN_PROGRESS — expanded Windows live delivery verified; mandatory resilience, unreachable-diagnostic, profile-specific LKG, and native Linux gates remain.
**Release target:** `v0.1.4a-next`
**Review date:** 2026-11-12
**Worktree:** `feat/v0.1.4a-next-work`, base worktree commit `59c85eed375c77653bcfcc57022ee01093467bc6`
**Publication:** No commit or PR has been made yet. This report does not claim PASS.

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

## Build and test gates

On the same Windows worktree, all passed:

```text
go test ./... -count=1
go vet ./...
go build ./...
GOOS=linux GOARCH=amd64 go build ./...
```

The Linux build above is a cross-build only. Ubuntu WSL2 is installed but has no Go toolchain; it is not a native Linux E2E host. Earlier WSL2 probes established Docker Desktop `alternate_port` behavior but do not substitute for this checkpoint's required native Linux Agent/Collector delivery run.

After the endpoint diagnostics, CLI inspection, and provider recovery tests were added, the full Windows suite, vet, build, and Linux/amd64 cross-build were rerun and passed.

## Remaining verification gates

These items remain open and prevent PASS:

- Native Linux unit/suite/vet/build and real Docker + pinned Collector + isolated OTLP E2E, including recreation and attribution.
- Actual Docker daemon outage followed by recovery and convergence of Agent inventory and OTel Observer. A new coordinator unit test covers provider failure followed by recovery and endpoint refresh, but it does not stop or isolate Docker Desktop and cannot establish receiver recovery.
- End-to-end diagnostics for every state. `agentctl inventory inspect` reports `endpoint_missing`, `endpoint_ambiguous`, and `endpoint_resolved`; resolved means reachability is unverified. A parser unit test accepts the pinned Receiver Creator dynamic error record and maps it to unreachable evidence. No live scrape failure was induced, and runtime Collector output is not yet routed into the Agent diagnostic surface.
- Nondefault private Redis port behavior. PR-05's initial Redis profile explicitly supports private port `6379`; a nondefault published host port is verified, but alternate private Redis ports are unsupported and were not tested.
- Multi-network receiver uniqueness and duplicate point behavior. The live run verifies two-network discovery and delivery, but Collector logs contain repeated starts for the same endpoint ID before a stop. Whether this produced overlapping receiver instances or duplicate points remains unresolved; do not mark single-receiver behavior PASS from these logs.
- The unpublished Windows target was discovered and diagnosed missing; a live no-receiver assertion is not retained.
- A profile-composed candidate validation/activation failure followed by LKG recovery and receiver ownership assertions. Existing generic candidate validation/LKG tests pass in the full suite, but the profile-specific failure injection was not run. No live activation failure was induced.
- Windows/Linux service-level behavior beyond the process-level live test.

Coordinator tests verify that failed observations preserve present inventory and a recovered snapshot clears the provider error and refreshes endpoint metadata. These use a fake provider and are unit evidence, not live Docker daemon recovery or Receiver Creator convergence. Do not infer the remaining behaviors from the successful Windows path.

## Changed scope and preservation

Agent Go source and tests, local architecture/plan/progress documentation, and this verification report are the intended PR-05 changes. No production configuration, schema, database migration, `docs/shared` content, or runtime artifact was added. The pre-existing untracked `cmd/agentctl/otel.yaml.lifecycle.lock` is preserved and is not part of PR-05. Temporary Collector/test artifacts are removed after verification.

**Checkpoint ruling:** Windows coverage includes multi-network observations, nondefault published host port, an unpublished target, inventory diagnosis, extensibility validation, and OTLP delivery. PR-05 remains **IN_PROGRESS / CONDITIONAL** because actual daemon recovery, live unreachable reporting, profile-composed LKG failure recovery/ownership, multi-network receiver uniqueness/duplicate points, nondefault private Redis ports, and native Linux E2E remain open. PR-06 remains blocked by PR-05.
