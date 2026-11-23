# OpenTelemetry-first integration strategy

**Decision direction:** OpenTelemetry Collector is the only telemetry execution engine. Agent is a management, metadata enrichment and configuration provisioning layer. Keep a pinned supported Collector distribution and verify its component manifest rather than assuming all upstream components are present.

## OTel responsibility

- **OTel:** observers, receiver_creator, native scraping receivers, processing, queues, exporters, OTLP.
- **Agent:** installation/supervision/LKG, host dependencies ownership, inventory/asset history, capability metadata, stable config generation/validation, health explanation.
- **Core:** policy publication and OpAMP transport; no per-container manual approval prompt in target DX.

## Dynamic endpoint plan

1. Docker Observer and Receiver Creator operate inside Collector for supported workloads/port metadata.
2. Lightweight Agent Docker Provider supplies enriched scoped observations and stable resource identity; it does **not** create runtime receiver instances for migrated targets.
3. Configuration Provisioner generates **rules for supported capabilities**, not a new config per Docker restart.
4. If a new capability is present and supported in pinned Collector, update configuration once and validate/activate; for instance churn, Observer/Receiver Creator handles it.
5. No rule/capability or unreachable endpoint → explain unsupported/unconfigured/unreachable. Never invent a collector plugin or pretend discovery alone equals successful scraping.
6. Existing Agent-owned Redis receiver materialization remains intact until parity on Windows/Linux proves migration safe. Never enable dual owner.

## Built-ins

`hostmetrics` always where supported. Keep Windows Exporter baseline and optional LHM + LHM Exporter, with install/inspect/repair/uninstall ownership; optional Node Exporter on Linux. Measure and avoid overlapping hostmetrics and exporter data. Collector Prometheus receiver scrapes enabled local exporters.

## Workloads and DX

| Target | Native path | Acceptance |
|---|---|---|
| Redis `redis:7-alpine` | `redis` receiver supported by pinned binary | Actual Redis metrics at isolated OTLP sink |
| PostgreSQL `postgres:17-alpine` | `postgresql` receiver with explicit creds | Actual DB metrics; missing creds handled safely |
| Python `python:3.12-alpine` | Generic Prometheus profile `/metrics` | Config-only; no Go build |
| Cooling/power mock | Generic Prometheus endpoint or external exporter | New provider/profile proof, no custom OTel receiver |
| Future SNMP/networking | Existing supported OTel receiver/exporter + source-specific metadata adapter | Design contract only, no universal auto-support claim |

## Risks/tests

Receiver Creator requires preconfigured matching rules and can spawn more than one receiver for overlapping templates. Collector binary may lack a component. Docker Desktop container addresses may not be reachable from Windows Agent. docker_observer can require privileged Docker socket; evaluate authentication, root-equivalent access, transport and namespace. Prove multi-instance identity, port changes, no-ghost convergence, no-op generations and real OTLP on both platforms before switching active ownership.

**Reference:** https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/receivercreator/README.md and https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/extension/observer/dockerobserver/README.md
