# PR-05 Linux live E2E evidence

**Date:** 2026-11-12
**Host:** GitHub-hosted `ubuntu-latest` Linux runner with its real Docker Engine
**Collector:** `otelcol-contrib 0.160.0`, Linux archive SHA-256 `7bb60c584c241c86261c2b8697cd3725dd8c56691f5ad5d98454eaa005b47b0c`
**Run:** [GitHub Actions 43238401049](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43238401049)
**Test:** `TestLiveNativeDockerRedisProfileDeliversOTLP` — PASS in 17.04s

The workflow downloads the version-pinned Collector archive, verifies the committed SHA-256, extracts `otelcol-contrib`, and sets the binary path for the real Docker/Collector/OTLP test. The Ubuntu job ran the Go suite, this live E2E, vet, and build successfully.

## Recorded scenario

- Docker assigned published host port `32770` to Redis private port `6379`.
- One published target was attached to two Docker networks.
- An opted-in Redis target without a published host port remained in Agent inventory; Linux used the profile's container-network endpoint strategy.
- Two mapped targets and the recreated target delivered Redis metrics to the isolated OTLP sink with expected Core `node_id`, container `resource_id`, `workload_id`, and container display-name attributes.
- Agent-generated Redis receiver instances remained absent for native-owned targets, and container churn did not change rendered config bytes or applied generation.
- Collector logs included Receiver Creator start/stop events and no scrape errors.

This run did not interrupt the Docker daemon, induce an ambiguous binding or scrape failure, or verify Windows behavior. It does not close the Windows multi-network repeated-start uniqueness question.
