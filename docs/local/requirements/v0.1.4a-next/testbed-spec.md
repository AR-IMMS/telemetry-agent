# Native Linux and Windows testbed

**Status:** PROPOSED. Prefer real hardware and native services; a VM is allowed only when identified as such. Do not count Docker-hosted Linux Agent on Windows as native Linux validation.

| Test node | OS | Agent execution | Container runtime | Required fixtures |
|---|---|---|---|---|
| L1 | Ubuntu 24.04 LTS x86_64 | systemd service | Native Docker Engine + Compose plugin | Redis, PostgreSQL, Python API |
| W1 | Windows 11/Server supported target x86_64 | Windows SCM | Docker Desktop Linux containers | Redis, PostgreSQL, Python API; Windows Exporter; LHM only if sensor support |
| Core/Gateway | Separate reachable host or VPS | NestJS OpAMP endpoint and auth | OTLP gateway + isolated verification sink | Logs, health/status and metric capture |

## Workload images and traffic

Use maintained/pinned digest versions after verification: `redis:7-alpine`, `postgres:17-alpine`, `python:3.12-alpine` as candidates, not immutable guarantees. Python fixture exposes `/metrics` and request generator. PostgreSQL uses a dedicated metrics user and appropriately protected credentials. No default passwords committed to repo. Optional mock cooling/power: a small exporter exposing Prometheus metrics, later test a real external device adapter if available.

## Network variants to cover

Container not published to host; custom bridge, multiple networks, recreated IP, Docker Desktop VM DNS/port-forwarding, exporter listening on host vs container, IPv4/IPv6 where available, TLS Gateway outage, proxy/firewall, unavailable Docker socket. A discovered container is not automatically a reachable scrape endpoint.

## Platform evidence

Capture `docker compose ps`, `docker inspect` selected metadata (redacted), service manager status, Agent diagnostics, Collector validation/logs, actual OTLP measurements at sink, config hashes/generations. On Linux run live native test; a Windows Docker Desktop networking failure is BLOCKED not PASS.
