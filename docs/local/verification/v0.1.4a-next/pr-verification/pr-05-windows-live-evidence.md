# PR-05 Windows live E2E evidence

## Closure rerun — current result

**Date:** 2026-11-12, Windows/amd64 with Docker Desktop Engine `29.5.3` and checksum-verified Collector `0.160.0`.
**Result:** `TestLiveNativeDockerRedisProfileDeliversOTLP` PASS in 15.73s.
**Scenario record:** host-published Redis private port 6379 mapped to host port `51564`; another Redis target used private port `6380` and host port `51566`; one unpublished opted-in Redis resource was attached to two Docker networks; three monitored Redis targets plus a recreated target delivered to the isolated OTLP sink.

The E2E now inspects Receiver Creator lifecycle events keyed by Collector instance and `endpoint_id`, failing if a receiver starts twice without a stop. It also checks for repeated `redis.clients.connected` points with the same resource and timestamp. Both assertions passed. Agent-owned Redis receiver definitions remained absent, and container churn left Collector config bytes and applied generation unchanged. Invalid port overrides are covered by profile diagnostics and registry tests.

This Windows rerun does not simulate Docker daemon failure and does not induce a live scrape failure; both require the native Linux isolated-Docker CI run. The multi-network unpublished resource verifies deterministic Agent diagnostic selection, while published receiver targets stay on a single network in this scenario. The reproducible scenario, assertions, and command are retained here.

The original run record below is retained as historical evidence for its older fixture. Its private Redis port remained 6379 and its repeat-start question was unresolved at that time.

**Date:** 2026-11-12
**Host:** Windows/amd64, Docker Desktop Engine `29.5.3` (`linux/x86_64`)
**Collector:** `otelcol-contrib 0.160.0`, binary SHA-256 `43A7D3EB2B759281B6297A91C830988BE39C16E538E8F55AEB4EA31AB7A612CD`
**Test:** `TestLiveNativeDockerRedisProfileDeliversOTLP` — PASS in 14.83s

## Reproduction

```powershell
$env:AR_IMMS_PR05_NATIVE_RUNTIME_E2E = '1'
$env:AR_IMMS_PR05_COLLECTOR_BINARY = '<checksum-verified otelcol-contrib 0.160.0 path>'
go test ./cmd/agentctl -run '^TestLiveNativeDockerRedisProfileDeliversOTLP$' -count=1 -v
```

The test creates disposable Redis containers and an isolated OTLP sink, starts the Agent runtime, observes actual inventory, waits for Redis metric delivery, removes/recreates a target, and removes its containers and test network on cleanup.

## Recorded scenario

- A published Redis target used Docker-assigned host port `59980` for private Redis port `6379`. This verifies a nondefault host mapping; the profile supports private Redis port `6379` only.
- That target was attached to two Docker networks.
- A third opted-in Redis target had no published host port and remained in Agent inventory. Windows profile diagnosis returned `endpoint_missing` for its supported host-published endpoint strategy.
- Two published targets and the recreated target delivered Redis metrics with Core `node_id`, distinct container `resource_id`, `workload_id`, and container display-name attributes.
- Agent desired Integration instances remained empty for native-owned targets; container churn did not change rendered Collector bytes or applied generation.
- Captured Collector output contained 5 Receiver Creator start records, 1 stop record, and no scrape errors. The log sequence repeated start records for the same endpoint ID before a stop. Delivery passed, but one active receiver per target and duplicate-point absence are not established.

The run did not induce an ambiguous published binding, a scrape failure, Docker daemon outage, candidate activation failure, or native Linux execution.
