# PR-05 Windows live E2E evidence

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
