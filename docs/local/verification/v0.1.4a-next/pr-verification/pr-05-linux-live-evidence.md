# PR-05 Linux live E2E evidence

## Final Linux recovery and endpoint diagnostics

**Run:** [GitHub Actions 43260591828](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43260591828), tested code commit `cfeca13`.
**Result:** PASS on `ubuntu-latest`, including full tests, isolated Docker/Collector/OTLP E2E, vet, and build.

The workflow started an isolated Docker-in-Docker engine with a persistent runner-local data root and shared API socket. The E2E recorded these assertions:

```text
PR05_DOCKER_RECOVERY preserved_inventory=true receiver_data_resumed=true applied_generation=2 collector_instances=2
PR05_ENDPOINT_DIAGNOSTIC resource=<matching Docker resource_id> state=endpoint_unreachable observed_at=<UTC timestamp> code=connection_failed
```

The test compared Collector instance identities before and after the daemon outage and required the sets to match; the reported total of two instances includes the earlier candidate activation in the scenario. It also required unchanged config bytes/applied generation and new OTLP points for every active target after restart. Redis pause induced a real receiver scrape failure; the Agent diagnostics correlated its timestamped evidence to the correct inventory resource. The same run passed the single-owner and duplicate-point checks.

Three earlier harness attempts failed and were retained as diagnostic evidence: run `43257539113` exposed socket permissions after restart; run `43258387022` exposed missing persistent Docker data; run `43259197716` exposed that the recovery set included a removed pre-churn resource. The fixes and final result are recorded in the [PR-05 report](./pr-05-report.md).

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
