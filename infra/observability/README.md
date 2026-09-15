# Local observability spine

Temporary home for the Phase 2A observability stack. This directory will
move to the backend/infra repository when the backend infrastructure boundary
is established.

## Scope

- OTLP/gRPC metrics ingress on port 4317
- OpenTelemetry Gateway
- Prometheus scrape storage
- Grafana with provisioned Prometheus datasource

Not included yet:

- TLS or node registration
- Loki, Tempo, alerting, dashboards
- analytics/business fan-out
- durable queueing or high availability

## Start

```bash
cp .env.example .env
```
