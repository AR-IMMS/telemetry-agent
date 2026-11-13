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
| E11a | Pinned OpAMP Supervisor capability + config composition | PASS / RUN 43400294752 | PARTIAL / LOCAL EVIDENCE | Ubuntu CI passed the enabled Supervisor profile fallback/reconnect/OTLP parity subcase for Redis/PostgreSQL/Prometheus/mock-cooling and uploaded evidence. This subcase runs the Supervisor directly after stopping Agent Runtime; it is not managed Agent profile parity. No ownership transfer. |
| E11b | Mock-backed OpAMP targeted update + status | PASS (run 43453791628) | PASS (run 43453791628) | Managed control E2E verifies versioned Gateway policy delivery, OpAMP identity binding, published/applied revision and hash attribution, restart recovery, rollback, and status convergence against Mock Core. Production NestJS Core interoperability remains separately unverified. |
| E11c | Managed enrollment + authenticated WSS + remote Gateway change | PASS (run 43453791628) | PASS (run 43453791628) | Exact report-updated head passed the full managed control E2E on both native runners, including enrollment, protected credential provisioning, authenticated WSS, policy changes, and OTLP delivery. See PR-07 report for scenario-specific evidence and artifacts. |
| E11e | Managed credential replacement, revocation, WSS reconnect + OTLP | PASS (run 43453791628) | PASS (run 43453791628) | Managed control E2E includes re-enrollment, protected credential replacement, rejection of the retired bearer, authenticated reconnect, and OTLP to updated sinks; the exact report-updated head passed on Windows and Ubuntu. |
| E11d | Managed Agent runtime application-profile parity | PASS (run 43453791628) | PASS / native elevated Windows, PR head `83e7074be0927dff40829e859a73b6585c720b33` | Windows Agent-managed Supervisor delivered Redis, PostgreSQL, Python/Prometheus, and mock-cooling metrics to the isolated OTLP sink. Container churn, identity continuity, receiver uniqueness, fallback, no duplicate telemetry, and cleanup passed. See [Windows evidence](pr-verification/pr-07-windows-profile-parity.md). |
| E12 | Core/Mock Core offline / reconnect / replay | PASS (run 43453791628) | PASS (run 43453791628) | Managed Agent/Supervisor starts before OpAMP availability, delivers fallback OTLP, then authenticates and converges after reconnect. Durable Mock Core publication/revision attribution and rollback evidence are recorded in the PR-07 report. |
| E13 | Invalid remote Collector config / crash / rollback | PASS / PARTIAL (run 43453791628) | PASS / PARTIAL (run 43453791628) | Managed Collector/Supervisor crash recovery, single replacement, process-tree cleanup, Agent restart, and LKG equivalence passed in the control E2E. Full invalid-candidate failure/diagnostic coverage remains tracked separately; do not infer it from crash recovery. |
| E13a | Single-owner migration and rollback | PASS (run 43453791628) | PASS (run 43453791628) | Native service E2E verifies PR-06 Agent install, managed Supervisor migration, restart, explicit rollback to legacy ownership, another restart, uninstall, and process/resource cleanup. See exact-head service artifacts in the PR-07 report. |
| E14 | Gateway unavailable then restored | NOT RUN | NOT RUN | errors + queue limits surfaced, no zero-loss claims |
| E15 | Read-only inspect/log/refresh | NOT RUN | NOT RUN | bounded output, right identity, no secret leak |
| E16 | Authorized restart + duplicate/stale/timeout | NOT RUN | NOT RUN | single action, durable outcomes; gated by security |
| E17 | SQLite migration/reopen/parallel CAS | NOT RUN | NOT RUN | old format preserved, correct transactions |
| E18 | Upgrade/uninstall with external exporter | NOT RUN | NOT RUN | no removal of unowned dependencies |

## Test dimensions

Docker Desktop VM vs native Linux network reachability; port unpublished/bridge/IPv6; mismatched image labels; unsupported capabilities; invalid credentials; profile collision; Docker daemon outage; event loss; write race; Collector process crash; OS reboot; protected sensors unavailable; TLS/certificate failures; multi Agent IDs; different Core target scoping.

Evidence rows added per execution: command/steps, OS, Agent and Supervisor commit/version, exact artifact URL/SHA/provenance, Collector version+hash, fixture digest, timestamp, status, failure details, logs and captured OTLP metric/resource attributes. Label Mock Core evidence separately from production Core E2E. Failed real E2E prevents declaring release DONE.
