# Local observability spine

The public telemetry endpoint is the TLS OTLP/HTTP JSON ingress on port 4317.
It checks each bearer with Fleet over mutual TLS and replaces client supplied
`agent_id` attributes before forwarding to the Collector Gateway. The Gateway
OTLP receiver is reachable only on the Compose network.

## Scope

- TLS OTLP/HTTP JSON metrics ingress on port 4317
- OpenTelemetry Gateway
- Prometheus scrape storage
- Grafana with provisioned Prometheus datasource

Not included yet:

- node registration
- Loki, Tempo, alerting, dashboards
- analytics/business fan-out
- durable queueing or high availability

## Start

```bash
cp .env.example .env
```

Set `OTLP_INGRESS_FLEET_ORIGIN` to Fleet's HTTPS origin. Fleet must trust the
ingress client certificate and set `FLEET_OTLP_INGRESS_CLIENT_CN` to its distinct
common name. The Agent Collector must trust the ingress server certificate.

Set `OTLP_INGRESS_CERT_DIR` to an absolute directory outside the repository
containing `server.crt`, `server.key`, `fleet-ca.crt`, `fleet-client.crt`, and
`fleet-client.key`. The image runs as an unprivileged user; grant that user
read access to these files without granting access to other users. Keep keys
out of `.env` and Compose configuration.

```bash
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d --build
```

Missing certificate material prevents startup. Fleet unavailability returns
HTTP 503 for new telemetry; invalid or revoked credentials return HTTP 401.
There is no positive authorization cache: revocation takes effect on the next
request after Fleet commits it. An in-flight request may finish within the
ingress's 10-second deadline. The Collector retries transient failures within
its configured bounds. Live deployment and end-to-end checks require Docker
access and provisioned certificates.
