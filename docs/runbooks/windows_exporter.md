# Windows Exporter Runbook

## Prerequisites

- Windows amd64.
- PowerShell running as **Administrator**.
- OTel Gateway reachable from the node.
- Current `agentctl` source/binary.

---

## Install or Reconcile

```powershell
go run ./cmd/agentctl dependency install windows-exporter

```

> This command reuses an existing healthy service if present. It does not automatically repair unhealthy services.

---

## Verify Local Exporter

```powershell
Get-Service windows_exporter

curl.exe -i http://127.0.0.1:9182/metrics

```

**Expected results:**

- Service is in the `Running` state.
- Endpoint returns HTTP `200`.
- Response contains Prometheus metrics (e.g., `windows_cpu_time_total`).

---

## Render and Validate Agent Config

```powershell
go run ./cmd/agentctl bootstrap `
  --config-root ./configs `
  --install-dir ./tmp/windows-install-hostmetrics `
  --config-path ./tmp/windows-config-hostmetrics/otel.yaml `
  --validation-endpoint 127.0.0.1:14317

```

**Check rendered config:**

```powershell
Get-Content ./tmp/windows-config-hostmetrics/otel.yaml

```

The metrics pipeline must contain:

- `otlp`
- `hostmetrics`
- `prometheus/windows_exporter`

---

## Run Agent

```powershell
go run ./cmd/agentctl run `
  --collector-path ./tmp/windows-install-hostmetrics/versions/otelcol-contrib-0.160.0-windows-amd64/otelcol-contrib.exe `
  --config-path ./tmp/windows-config-hostmetrics/otel.yaml `
  --gateway-endpoint 127.0.0.1:14317

```

> Stop the foreground Agent using `Ctrl+C`; the supervisor will trigger a graceful Collector shutdown.

---

## Verify Central Ingestion

From the `infra/observability` directory:

```bash
docker compose exec prometheus \
  wget -qO- \
  'http://localhost:9090/api/v1/query?query=windows_cpu_time_total'

```

**Expected success response includes labels such as:**

- `exported_instance="127.0.0.1:9182"`
- `host_id="windows-..."`
- `host_name="..."`

---

## Common Failures

| Symptom                   | Action                                                                    |
| ------------------------- | ------------------------------------------------------------------------- |
| **Administrator error**   | Open PowerShell using **Run as Administrator**                            |
| **Service absent**        | Re-run `dependency install`                                               |
| **Service unhealthy**     | Check service logs and endpoint; do not automatically overwrite/reinstall |
| **Agent fails to export** | Verify Gateway endpoint and Gateway health                                |
