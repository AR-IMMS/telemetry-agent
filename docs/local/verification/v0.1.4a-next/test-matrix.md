# Platform E2E test matrix — v0.1.4a-next

**Status:** PLANNED; all tests below are `NOT RUN` unless verified later by a real test report. Detailed historic 100-case matrix remains in `architecture-overview.md` §14; do not mark historical evidence as current release proof.

| ID | Scenario | Linux | Windows | Critical expected evidence |
|---|---|---|---|---|
| E01 | Native install / system manager / reboot | NOT RUN | NOT RUN | Agent + Collector restore, only one managed process |
| E02 | Enrollment, lost-response, revoke, re-enroll | NOT RUN | NOT RUN | Correct binding, secure secret, reject revoked |
| E03 | Baseline host telemetry & exporter ownership | NOT RUN | NOT RUN | Actual OTLP metrics, Windows Exporter; optional LHM conditional |
| E04 | Redis Alpine auto monitoring | NOT RUN | NOT RUN | Native redis receiver metrics at sink |
| E05 | PostgreSQL Alpine with credentials | NOT RUN | NOT RUN | Actual db metrics, restricted access; missing creds negative |
| E06 | Python Alpine `/metrics` generic profile | NOT RUN | NOT RUN | Prometheus metrics with no Agent rebuild |
| E07 | Docker add/remove/recreate/scale/port shift | NOT RUN | NOT RUN | correct identities, no ghost after convergence |
| E08 | Observer + Receiver Creator owner migration | NOT RUN | NOT RUN | Actual pinned components; no duplicate scrape |
| E09 | Discovery provider error / partial snapshot | NOT RUN | NOT RUN | no false resource deletion; recovery |
| E10 | Mock cooling or power profile/provider | NOT RUN | NOT RUN | new adapter without engine changes |
| E11a | Pinned OpAMP Supervisor capability + config composition | NOT RUN | NOT RUN | Partial Windows evidence is in the [PR-07 report](pr-verification/pr-07-report.md): exact artifact, fallback, local+remote composition, APPLIED status, invalid candidate reporting, automatic rollback, Collector crash restart, and OTLP delivery passed. Existing Redis/PostgreSQL/Prometheus profiles and native Linux remain NOT RUN. No ownership transfer. |
| E11b | Mock-backed OpAMP targeted update + status | NOT RUN | NOT RUN | contract revision/hash, identity binding, apply/failure mapping; mock result only |
| E11c | Managed enrollment + authenticated WSS + remote Gateway change | NOT RUN | NOT RUN | Initial CI attempt exposed and fixed missing identity initialization; second attempt reached Collector config validation and found a literal YAML tab. That fixture tab is removed; rerun both CI jobs and retain sanitized evidence before recording PASS. |
| E12 | Core/Mock Core offline / reconnect / replay | NOT RUN | NOT RUN | cached/fallback config, no duplicate activation, reconverges after restart |
| E13 | Invalid remote Collector config / crash / rollback | NOT RUN | NOT RUN | actual rollback setting and evidence, LKG preserved, diagnostics truthful |
| E13a | Single-owner migration and rollback | NOT RUN | NOT RUN | no overlapping lifecycle owners; reversible transfer after parity |
| E14 | Gateway unavailable then restored | NOT RUN | NOT RUN | errors + queue limits surfaced, no zero-loss claims |
| E15 | Read-only inspect/log/refresh | NOT RUN | NOT RUN | bounded output, right identity, no secret leak |
| E16 | Authorized restart + duplicate/stale/timeout | NOT RUN | NOT RUN | single action, durable outcomes; gated by security |
| E17 | SQLite migration/reopen/parallel CAS | NOT RUN | NOT RUN | old format preserved, correct transactions |
| E18 | Upgrade/uninstall with external exporter | NOT RUN | NOT RUN | no removal of unowned dependencies |

## Test dimensions

Docker Desktop VM vs native Linux network reachability; port unpublished/bridge/IPv6; mismatched image labels; unsupported capabilities; invalid credentials; profile collision; Docker daemon outage; event loss; write race; Collector process crash; OS reboot; protected sensors unavailable; TLS/certificate failures; multi Agent IDs; different Core target scoping.

Evidence rows added per execution: command/steps, OS, Agent and Supervisor commit/version, exact artifact URL/SHA/provenance, Collector version+hash, fixture digest, timestamp, status, failure details, logs and captured OTLP metric/resource attributes. Label Mock Core evidence separately from production Core E2E. Failed real E2E prevents declaring release DONE.
