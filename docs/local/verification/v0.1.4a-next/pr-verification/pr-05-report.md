# PR-05 verification report — OTel-native Docker/Redis discovery

**Status:** IN_PROGRESS — final Windows verification passes for receiver uniqueness, OTLP duplication, private-port override, and endpoint failure reporting. The mandatory isolated native Linux Docker outage/recovery and live endpoint-failure gates are pending the updated draft-PR CI run.
**Release target:** `v0.1.4a-next`
**Review date:** 2026-11-12
**Worktree:** `feat/v0.1.4a-next-work`, base worktree commit `59c85eed375c77653bcfcc57022ee01093467bc6`
**Implementation commit:** `0f03c25` (`feat(discovery): add OTel-native Docker Redis monitoring`)
**Publication:** Draft [PR #28](https://github.com/AR-IMMS/telemetry-agent/pull/28) targets `chore/v0.1.4a-next`. This report does not claim PASS.
**Previously verified PR head:** `300d6b1`; GitHub Actions run [43238401049](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43238401049) passed on Windows and Ubuntu, including Linux live Docker/Collector/OTLP E2E. Final closure changes are being prepared for the draft PR and require a fresh CI run.
**Linux live E2E:** GitHub Actions run [43238401049](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43238401049) passed on `ubuntu-latest`, including the real Docker/Collector/OTLP profile E2E.

## Accepted contract implemented

This checkpoint implements the user-approved PR-05 contract in [discovery-monitoring-contracts.md](../../../architecture/v0.1.4a-next/discovery-monitoring-contracts.md): `node_id` comes from the registered Core binding; Docker container ID is `resource_id`; `workload_id` is optional; container name is display metadata. Docker/Redis monitoring is opt-in through the three approved labels. Agent inventory and policy remain Agent-owned, while OTel Observer and Receiver Creator own the dynamic Redis receiver for matching targets. Unlabelled/manual targets keep the existing Agent receiver path. Inventory churn does not change the native profile's rendered configuration or activate a new generation.

The native layer is enabled only when a Docker endpoint, registered Node ID, and pinned Redis capability are available. It reuses existing Collector candidate validation, activation, readiness, and LKG recovery paths; no database migration or shared-submodule edit was made. Provider label mapping is allowlisted. Incomplete/provider-error inventory snapshots retain prior resources.

## Windows evidence

Environment: Windows/amd64, Go `go1.25.5 windows/amd64`, Docker Desktop Engine `29.5.3` (`linux/x86_64`), Collector `otelcol-contrib 0.160.0` (SHA-256 `43A7D3EB2B759281B6297A91C830988BE39C16E538E8F55AEB4EA31AB7A612CD`). The live test used disposable Redis 7 containers, the real local Docker Engine, the Agent runtime, the pinned Collector, and an isolated OTLP receiver/sink.

| Evidence level | Result | Evidence |
|---|---|---|
| Discovered | PASS | The Agent inventory contained two opted-in published containers and one opted-in container without a published host port, all with distinct container IDs and present state. |
| Receiver active | PASS | The latest run parses Receiver Creator start/stop records by Collector instance and endpoint ID. It failed on a duplicate active start; the observed sequence passed with one active receiver per observed target. |
| Collection verified | PASS | `redis.clients.connected` points were observed for both initial instances and for the recreated instance. |
| Delivery verified | PASS | The isolated OTLP sink received points carrying the expected Core `node_id`, distinct `resource_id` values, shared `workload_id=cache-service`, and display container name. |
| Single owner / churn | PASS (Windows) | Agent desired instances remained empty and no static Agent Redis receiver was generated. Removal/recreation left applied generation and config bytes unchanged. The lifecycle parser and OTLP point counter found no duplicate receiver or duplicate point in the run. |
| Endpoint cases | PARTIAL | The latest run covered two published Redis targets, private port override `6380`, a nondefault host mapping, and an unpublished multi-network resource; profile diagnostics select highest `GwPriority`, then network name. The unpublished target is `endpoint_missing`; invalid overrides are rejected. The updated native Linux run must still verify live `endpoint_unreachable` and daemon recovery. |

Executed live command:

```powershell
$env:AR_IMMS_PR05_NATIVE_RUNTIME_E2E = '1'
$env:AR_IMMS_PR05_COLLECTOR_BINARY = '<checksum-verified otelcol-contrib 0.160.0 path>'
go test ./cmd/agentctl -run '^TestLiveNativeDockerRedisProfileDeliversOTLP$' -count=1 -v
```

Latest closure result: PASS (15.73 seconds). Scenario log recorded host port `51564`, explicit private port `6380` mapped to `51566`, two networks on the unpublished resource, three simultaneously monitored Redis targets, recreation, one active receiver per Collector instance/endpoint, and no duplicate `redis.clients.connected` point at an identical timestamp. The test removes its disposable containers and network. Earlier exploratory runs with `use_host_bindings: true` failed to create matching receivers; that option was reverted because it changes the Docker Observer/Receiver Creator variable behavior and is not needed for the accepted `alternate_port` strategy. The separate pinned-binary profile configuration validation also passed.

## Closure implementation and remaining gates

- Docker inventory now preserves network name and `GwPriority`; diagnostic selection mirrors the pinned Observer's priority/name ordering. Docker Observer stays on the documented default `use_host_bindings: false` behavior for the host `alternate_port` strategy.
- Redis target private port defaults to 6379 and accepts `ar-imms.monitoring.port` as an ASCII decimal integer from 1 through 65535. Invalid values report `endpoint_invalid` and do not match the Receiver Creator rule. The provider records this label without profile-specific interpretation.
- Collector network scrape errors are reduced to receiver ID, endpoint, and normalized error code; the Agent status API retains at most 32 failures. `agentctl inventory inspect` correlates matching evidence and reports `endpoint_unreachable`; raw Collector log lines are not retained by the production status path.
- A native Linux CI harness starts an isolated Docker-in-Docker engine and interrupts only that engine. Inventory retention, successful post-recovery delivery, receiver convergence without Collector restart, and live endpoint-failure correlation remain unverified until this workflow revision passes.
- The CI Windows polling test failed once at its one-second startup-reconciliation wait in run `43239629059`; its CI retry and a fresh normal-access `-count=100` run both passed. The evidence supports runner scheduling sensitivity; no product regression reproduced and no code change was justified.

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

After the final endpoint diagnostics and helper refactor, the host-permission full Windows suite, vet, build, Linux/amd64 cross-build, and architecture guard all passed. The SQLite polling regression ran 100 times and passed. A sandboxed test attempt failed to create SQLite files and bind loopback sockets due to sandbox restrictions; it was discarded and the same full suite passed with host permissions.

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

- Stop and restart the isolated Docker daemon on the native Linux runner; verify failed observations preserve inventory, post-recovery discovery and OTLP delivery converge, and the Collector instance/config generation does not restart or change unnecessarily.
- Induce an actual receiver scrape failure on native Linux and verify timestamped Collector evidence appears through `agentctl inventory inspect` as `endpoint_unreachable` for the matching resource. `endpoint_missing`, `endpoint_ambiguous`, and `endpoint_resolved` are covered by profile diagnostics; `resolved` means reachability is unverified.
- Windows/Linux service-manager behavior beyond the process-level live tests is not covered by this checkpoint.

Coordinator tests cover provider error and recovery transitions, but they are unit evidence and do not replace the isolated live daemon test.

## Changed scope and preservation

Agent Go source and tests, local architecture/plan/progress documentation, and this verification report are the intended PR-05 changes. No database migration or `docs/shared` content was added. The pre-existing untracked `cmd/agentctl/otel.yaml.lifecycle.lock` is preserved and excluded from PR-05. Temporary Collector/test artifacts are excluded from Git and removed after verification; reproducible commands and scenario evidence are retained in this report and [Windows live evidence](./pr-05-windows-live-evidence.md).

**Checkpoint ruling:** Windows live delivery verifies private port 6380, one active Receiver Creator per target, and no duplicate OTLP points. The status/reporting path now carries sanitized, timestamped Collector scrape failures into `inventory inspect`. PR-05 remains **IN_PROGRESS / CONDITIONAL** until updated native Linux CI verifies isolated Docker outage/recovery, inventory retention and refresh, receiver convergence without Collector restart, post-recovery OTLP delivery, and live `endpoint_unreachable`. PR-06 remains blocked by PR-05.
