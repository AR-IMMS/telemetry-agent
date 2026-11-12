# Release acceptance criteria

**Status:** PROPOSED. This is a gate contract, not test results.

| Gate | Reproducible acceptance evidence | Release decision |
|---|---|---|
| A1 Install | Native installer/service lifecycle, reboot and upgrade on both OSes; no second Agent/Collector process | Mandatory |
| A2 Enrollment | Real Core enrolment, binding, invalid token, duplicate/recovery, revoked credential checks | Mandatory |
| A3 Host | Actual host telemetry verified at isolated OTLP sink; Windows Exporter validated, LHM conditional on supported hardware | Mandatory + conditional hardware |
| A4 Applications | Redis native metrics, PostgreSQL native metrics with configured credentials, Python Prometheus metrics on real Docker | Mandatory |
| A5 Lifecycle | Add/remove/recreate/scale, no ghost after documented convergence, multi-instance identity, stable no-op generation | Mandatory |
| A6 Developer DX | New Python /metrics and mock facilities endpoint via profile/adapter without Agent Go changes or rebuild | Mandatory |
| A7 OTel delegation | Component manifest verified; observer/receiver_creator active; only one receiver owner; parity with Redis baseline | Mandatory |
| A8 Remote config | Core NestJS OpAMP HTTP↔Go client: targeted policy, invalid payload, hash/status, reconnect, no-op, rollback | Mandatory |
| A9 Health/recovery | Agent crash, Collector crash, Gateway outage, invalid config, startup recovery; no false delivery claim | Mandatory |
| A10 Operations | Correct read-only diagnostics; any enabled mutating ops prove RBAC, exact identity, durable idempotency, audit, timeout/unknown | Mandatory within approved scope |
| A11 Compatibility | Previous SQLite fixtures, public CLI contracts, managed dependencies, same Redis metrics on Windows/Linux | Mandatory |
| A12 Security | Secrets redaction, TLS/auth, stale target, Docker socket exposure risk, untrusted metadata/profile, rollback/revocation | Mandatory |

## Evidence format

Each test: platform, OS build/architecture, testbed, Agent/Collector version+digest, Git commit, exact steps, raw logs/artifacts, metric names/resource attributes, actual result, PASS/FAIL/BLOCKED/NOT RUN/UNSUPPORTED, known limitations. `go test` PASS does not substitute for hardware E2E. Collect network reachability and Docker Desktop VM constraints explicitly.

## Important negative tests

Unknown service must not generate a fictional receiver; receiver_creator cannot load a missing type. Failed/partial provider snapshots must not mark resources deleted. Same hash must not needlessly restart Collector. In-flight operation timeout is `UNKNOWN` unless outcome evidence exists; retries cannot accidentally repeat restarts. A config marked applied means validated runtime readiness, not proved backend delivery.
