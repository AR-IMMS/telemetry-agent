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

## PR-09.5 source-ownership behavior comparison

This is a scoped before/after comparison for the consolidation changes; it does not change the release-level E01–E18 statuses above. Baseline is merged PR #38 at `5a06d7d6fe8ac6c2ba09043ad1fd6f3a769e1ce8`; PR-09.5g source is `3c87b8e1882f1beffa4a07a1eb610822cfbe5200`.

PR #39 implementation/workflow head `0a90b9252d99981fd837a85f03e48f29d1eff227` passed Windows/Ubuntu CI [43839603999](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43839603999). Both runners passed full tests, focused race checks and native service lifecycle E2E; Ubuntu additionally passed the live Docker, JetStream, OTLP profile and managed Supervisor checks listed in the [verification report](pr-verification/pr-09.5-report.md). Windows live Docker/JetStream/profile steps were skipped by workflow conditions. Later report/architecture/tracker edits are documentation-only and receive CI on PR #39 without changing tested Go or workflow files.

The report-only PR head `de0cf5ca692cb5280460fffaa381f967e1df751c` also passed both CI jobs in [43841832018](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43841832018). This closeout entry is documentation-only; the executable source and workflow remain unchanged.

| Behavior / boundary | Before | After | Evidence / invariant |
|---|---|---|---|
| Integration enable/update/retry decisions | CLI coordinated writes and retry freshness checks | Configuration owns these decisions; CLI composes the use case | PR-09.5f source tests and final-head CI [43833504369](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43833504369); flags, CAS, no-op revision and omitted-enabled preservation retained |
| Discovery-cycle completion | Application Host inferred completion semantics | Inventory returns cycle result; Application Host coordinates worker lifecycle | PR-09.5f source tests and final-head CI [43833504369](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43833504369); provider errors do not count as successful complete observations |
| Collector capabilities | Monitoring composition could reach concrete runtime catalog | Monitoring queries Runtime's pinned catalog through a narrow contract | PR-09.5f source tests/CI; one catalog owner and no duplicated capability list |
| Monitoring profile ownership and Collector projection | Profile definitions, YAML loading, endpoint diagnostics and Collector YAML projection shared `internal/monitoringprofile` | Monitoring owns profile registry, YAML source and endpoint diagnostics; Runtime owns Collector projection through a Runtime contract translated at composition | Source commit `82666c4` passes local Windows full suite/vet/build/architecture guard; no hosted CI or live OTLP retest claimed yet; existing receiver rules, credentials handling and identity attributes retained |
| Identity SQLite entrypoints | `agentstate` forwarding facades exposed Identity writes | Identity repositories are used directly, including in test fixtures | 9.5g commit `62ab91f`; no production facade caller, schema or data change |
| Dependency uninstall coordination | CLI rebuilt Collector activation inputs only to pass an ignored parameter | Runtime request is made without dead activation composition | 9.5g commit `3c87b8e`; persisted-context validation, durable teardown intent, service teardown and lifecycle owner retained |
| Persisted schema, Collector/LKG, Supervisor ownership, Operations journal/outbox | Existing context-owned behavior | Unchanged in 9.5g | Full Go suite and focused race tests at `3c87b8e`; no migration or persisted-format edits |

The above is a focused source disposition check, not a claim that every historical release acceptance scenario was rerun on both native platforms. Current-head CI results and platform-specific skips are recorded in the [PR-09.5 verification report](pr-verification/pr-09.5-report.md).
