# Release scope — v0.1.4a-next

**Status:** PROPOSED | **Product promise:** install once, enroll, monitor host and supported application services, extend monitoring with native OTel capabilities, remotely update configuration, inspect health and perform limited secure operations on native Linux/Windows.

## Must ship (P0 release contract)

| ID | Capability | Concrete outcome |
|---|---|---|
| R01 | Native install/upgrade/remove | `systemd` on Linux and Windows SCM; restart/reinstall retains identity; ownership-safe cleanup |
| R02 | Identity/enrollment | Durable installation binding, protected credentials, reconnect and revocation enforcement |
| R03 | Host monitoring | OTel hostmetrics; preserve Windows Exporter and optional LHM/LHM Exporter; optional Linux Node Exporter; actual telemetry to Gateway |
| R04 | App/service monitoring | Redis and PostgreSQL containers produce receiver-validated real metrics; Python `/metrics` via generic Prometheus profile; explicit handling of credentials and image variants |
| R05 | Dynamic lifecycle | Add/stop/recreate/scale Docker services; no ghost receivers after convergence; identities remain distinguishable |
| R06 | OTel-first extensibility | Observer + Receiver Creator for supported dynamic targets; receiver type validated against pinned Collector; no proprietary receiver engine |
| R07 | Lightweight discovery support | Provider/listener interface, normalized registry, provenance, capability hints; Docker implementation; mock cooling/power adapter proof |
| R08 | Central management | NestJS OpAMP HTTP adapter + Go client; targeted versioned remote config; actual apply/failure status |
| R09 | Resilience | SQLite, start/reconcile, Collector validation/readiness/LKG, offline continuation and accurate errors |
| R10 | Self-health | Agent, Collector and pipeline status separated; export evidence not inferred from readiness |
| R11 | Safe operations | Read-only inspect/logs/refresh; scoped container restart and service restart only if enforceable authorization and privilege design passes security gate |
| R12 | Compatibility | Existing enrollment, CLI, SQLite records, Redis E2E and dependency ownership preserved through incremental migration |

## Scope limit / exclusions

No SSH, arbitrary remote command, autonomous AI remediation, automatic secret discovery, full OpAMP functionality, guaranteed zero telemetry loss, arbitrary unknown-protocol monitoring, or physical cooling/power hardware integration. Native PostgreSQL scraping requires valid credentials and reachable network endpoint; mere discovery is not success.

## Product DX / policy

No per-discovered-service approval prompt. Supported trusted **preconfigured** monitoring paths should converge automatically. This does not waive security or authorization for secrets, privileged endpoints, dependencies, or mutating actions. Core-vs-local configuration precedence remains **OPEN** and must have its own ADR before broad remote rollout.

## Release labels

Use `PASS`, `FAIL`, `BLOCKED`, `NOT RUN`, `UNSUPPORTED` per platform and hardware; never aggregate Windows and Linux evidence into a single unqualified PASS. If a MUST capability fails, do not claim the release is complete. `R11` mutating actions cannot be shipped enabled without the security gate.
