# Development: Central Metrics Spine

## Goal

Provide a working metrics path from an AR-IMMS node agent to Grafana through
an OTel Gateway and Prometheus.

## Configuration ownership

| Location                                                               | Ownership                                                                      |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `configs/base/otel.yaml`                                               | Shared Agent OTLP export, identity processor, and plaintext homelab transport. |
| `configs/os/linux/otel.yaml`                                           | Linux hostmetrics receiver and metrics pipeline membership.                    |
| `configs/os/windows/otel.yaml`                                         | Windows hostmetrics receiver and metrics pipeline membership.                  |
| `infra/observability/otel-gateway/gateway-config.yaml`                 | Gateway OTLP ingress and Prometheus exporter.                                  |
| `infra/observability/prometheus/prometheus.yml`                        | Gateway scrape target.                                                         |
| `infra/observability/grafana/provisioning/datasources/prometheus.yaml` | Provisioned Grafana Prometheus datasource.                                     |

## Hostmetrics baseline

The Agent configures `hostmetrics` with a `15s` collection interval and these
scrapers:

```text
cpu
memory
disk
filesystem
network
paging
```

The `metrics` pipeline contains:

```text
otlp
hostmetrics
```

Windows exporter is intentionally excluded from the baseline because the Agent
does not yet install, configure, or supervise that external service.

## Acceptance criteria

* Rendered Linux and Windows configurations contain the hostmetrics baseline.
* Rendered Agent configuration sets `exporters.otlp/gateway.tls.insecure: true`
  for the homelab PoC.
* Bootstrap validates the rendered configuration with the pinned Collector.
* The Agent starts, becomes healthy, and starts the hostmetrics receiver.
* Prometheus reports the Gateway target as `UP`.
* Gateway `/metrics` exposes `system_*` metrics with `host_id`, `host_name`,
  and `service_name` labels.
* Grafana uses the provisioned Prometheus datasource.

## Verification commands

```powershell
go test ./internal/config -v
go test ./...
go vet ./...
go build ./...
```

```powershell
docker compose up -d
docker compose ps
```

```powershell
go run ./cmd/agentctl bootstrap `
  --config-root ./configs `
  --install-dir ./tmp/windows-install-hostmetrics `
  --config-path ./tmp/windows-config-hostmetrics/otel.yaml `
  --validation-endpoint 127.0.0.1:14317
```

```powershell
go run ./cmd/agentctl run `
  --collector-path ./tmp/windows-install-hostmetrics/versions/otelcol-contrib-0.160.0-windows-amd64/otelcol-contrib.exe `
  --config-path ./tmp/windows-config-hostmetrics/otel.yaml `
  --gateway-endpoint 127.0.0.1:14317
```

## Deferred work

* Validate the same runtime path on native Linux or WSL.
* Install and supervise Windows exporter.
* Install and configure LibreHardwareMonitor.
* Replace deprecated Collector component aliases.
* Add TLS, mTLS, node registration, queueing, dashboards, logs, and traces.
