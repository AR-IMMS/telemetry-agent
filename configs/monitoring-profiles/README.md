# Local Docker monitoring profiles

Place versioned Agent profile files directly in this directory. The Agent reads
only top-level `.yaml` and `.yml` files from the configured root's
`monitoring-profiles/` directory. Files under `examples/` are reference
templates and are not loaded automatically.

Profiles use `ar-imms/v1alpha1`, exact Docker opt-in labels, a supported native
receiver/capability pair, a private target port, and typed receiver settings.
The Agent validates the complete profile registry before generating the OTel
Docker Observer and Receiver Creator configuration. Raw Collector configuration
and Receiver Creator expressions are not accepted in profile files.

Copy and edit [python-metrics.yaml](examples/python-metrics.yaml) to monitor a
Prometheus `/metrics` endpoint, or [postgresql.yaml](examples/postgresql.yaml)
to monitor PostgreSQL. PostgreSQL files reference environment variable names;
they must not contain credential values. The Collector process must receive
the referenced values through the operator's local service environment before
the profile is activated. `tls_ca_file` can point to the local CA certificate
when the database uses a private trust chain; profiles do not expose an
insecure TLS bypass.

Example workload opt-in:

```yaml
labels:
  ar-imms.monitoring.enabled: "true"
  ar-imms.monitoring.profile: python-metrics
  ar-imms.monitoring.port: "9108" # optional; profile default otherwise
  ar-imms.workload.id: api-service # optional logical identity
```

Adding a new Prometheus workload profile is a data-only operation once its
typed profile file is installed. It does not change generic discovery logic or
introduce another scraper.
