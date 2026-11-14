# PR-08 Health & Reporting Verification

- **Status:** IN_PROGRESS
- **Branch:** `feat/pr-08-health-reporting`
- **Base:** PR-07 merge `700768dff651e73a2e457aa20d94845bc326d971`
- **Latest committed implementation head:** `7f274f3d486a9754e6452702ad969f9eb09e6ff1`; additional review fixes are currently uncommitted in the isolated PR-08 worktree and require a new exact-head CI run before merge.
- **Implementation commits:** `b634eed40ae7f7b6996384f1370c7ec1994e31ab`, `13398ced1c1d16de992025f8927636d3bc3695e2`, `082f7535047f393108690d0c64c0caf31b76efa4`, `4ab5958b344cd02330b04142a7ef53cad3b4b667`, `7a2601869863fc7a8e6f82c7feed886a40fdbf4a`, `0e0bc19` (runtime evidence attribution and safe status errors), `7f274f3` (bounded owner evidence)
- **PR:** draft [#31](https://github.com/AR-IMMS/telemetry-agent/pull/31), targeting `chore/v0.1.4a-next`.

## Goal and boundaries

Provide truthful local Agent health evidence and diagnostics. Health projects evidence owned by Runtime, Inventory, Monitoring, Configuration, dependencies, and supported connectivity adapters; it does not start/restart processes, mutate config, or claim backend delivery from Collector readiness or OpAMP state. Core health-report transmission remains unimplemented until an approved versioned Core contract defines authentication, status vocabulary, retries, idempotency, retention, and replay.

The source tree already had a local `/v1/status` API, a `status` CLI command, and an `internal/agenthealth` package. PR-08 extends those components instead of adding another reporting pipeline.

## Source-checked evidence matrix

| Dimension | Current owner/source | Available evidence | Freshness and completeness | PR-08 projection |
|---|---|---|---|---|
| Agent runtime | Local status HTTP handler | A successful response proves the Agent status endpoint handled a request | Generated for the current response | `healthy`, response timestamp |
| Core connectivity | Identity/enrollment and Core adapter | Enrollment outcome exists; no current connectivity probe or health-report contract | Current connectivity is unknown | `unknown: core_connectivity_not_observed` |
| OpAMP Supervisor | Runtime managed-owner path | Persisted managed-owner config; readiness callback only after managed Collector readiness | No separate OpAMP connection assertion; runtime event time only | `unsupported` for legacy owner; managed readiness evidence otherwise |
| Collector | Runtime watcher and Collector readiness probe | Lifecycle state, desired/activated/applied generations, pending teardown count; sanitized structured scrape-failure observations | Collector/Supervisor completeness requires an observed lifecycle state; freshness needs owner-provided `validUntil` | Healthy/degraded/unknown state, component-specific completeness and explicit freshness |
| Configuration | Configuration/Runtime state | Generation equality and pending teardowns; applied means generation-bound runtime readiness | Completeness requires the runtime state/generation observation; timestamp alone is partial; no implicit age policy | Healthy only for equal nonzero generations with no pending teardown; otherwise degraded/unknown |
| Inventory | Inventory SQLite repository | Provider enabled state, last attempt/success, error code, and resource count; Health reads at most 64 provider rows and never loads the resource inventory | Uses provider timestamps and Discovery's `DefaultPolicy().StaleAfter` (two minutes); provider error makes the projection partial; a truncated read remains unknown/partial | Latest provider failure is `degraded`; all enabled providers with successful snapshots are `healthy`; missing evidence is `unknown`; stale last success is marked stale |
| Monitoring | Monitoring profile and endpoint diagnostics | Profile match, endpoint resolution, and timestamped Collector scrape failure; no success counter proving current collection | Failure evidence has source timestamp; no success inference from absence of failures | An observed scrape failure marks collection degraded with unknown freshness; no failure and no success evidence remains unknown |
| Dependencies | Dependency status adapters | Read-only availability, health, drift, and per-inspector failure projection from the existing `dependency status` service | Queried for each health snapshot with a five-second overall context deadline; freshness is request-scoped | Enabled dependencies all healthy is `healthy`; unhealthy/drifted or failed inspection is `degraded`; incomplete evidence is `unknown`; no enabled dependencies is `unsupported` |
| Telemetry collection | OpenTelemetry Collector | A matching receiver failure can prove a collection attempt failed; readiness or lack of failure cannot prove scraping | Failure is timestamped; no verified-success freshness source is available | `degraded` with unknown freshness on explicit failure; otherwise `unknown`; successful collection remains unverified |
| Telemetry delivery | Collector/export path | Actual sink receipt is verified only by isolated E2E fixtures; Agent runtime has no production receipt signal | No production freshness/completeness source | `unknown: backend_receipt_not_measured` |

### Contract and conflict findings

- Existing top-level `Snapshot.Status` reflects Collector readiness, applied configuration generations, pending teardowns, and the last runtime error. It is not a full system health result. Existing keys remain for compatibility; the new assessment is additive. A nil reporter retains the existing degraded top-level status while its detailed evidence is unknown.
- Existing Collector network failures are reduced to receiver, host/port, code, and observation time; raw Collector log lines are not retained. The list is bounded at 32, but previously accepted identifier strings had no length limit and `LastError` was exposed verbatim. PR-08 bounds/redacts those diagnostics.
- Readiness and a successful config-generation application are not proof of scrape or delivery. `doctor` shows those as separate dimensions; explicit scrape failures mark collection degraded, while absence of both success and failure evidence remains unknown. Delivery remains unknown without sink receipt.
- Health consumes Inventory and Dependency projections through read-only sources composed at the `agentctl` boundary. Inventory reads only provider-status rows through Discovery's capped `ProviderStatusReader` (64 maximum) and uses the Discovery-owned two-minute stale policy. Dependency inspection is bounded by a five-second request deadline; neither adapter mutates owner state or starts a separate polling loop.
- The HTTP request context now reaches both owner evidence sources. Inventory provider-status reads accept that context and SQLite checks cancellation before opening the database and uses `QueryContext`; Dependency derives its bounded timeout from the same caller context. The inventory read remains capped at 64 rows.
- A failed Agent-state read now degrades the Agent runtime dimension while leaving Collector, Supervisor ownership, and configuration unknown. An observed Collector failure remains degraded. This avoids turning an Agent database/read error into a fabricated Collector failure or a false legacy-ownership result.
- No approved Core reporting endpoint or wire contract is present in the local contracts. No Core route, persistence model, retry policy, or status mapping is invented.
- No global health evidence TTL is defined. PR-08 uses explicit owner-provided validity deadlines; missing observation/validity data remains unknown.
- Independent review identified a nil-reporter compatibility status regression and timestamp-only completeness inference; both were corrected in `13398ce` and covered by focused regression tests.

## PR-07 Windows verification retry

The PR-07 gate is already closed and PR #30 is merged. A fresh genuine RunAs/UAC run was repeated against merge head `700768dff651e73a2e457aa20d94845bc326d971`; the PR head source code is identical for the test/runtime files. The elevated token reported High Mandatory Level, enabled Administrators SID, and Administrator role. Docker Desktop `desktop-linux` connected to Linux Engine 29.5.3 x86_64.

Command: `go test ./cmd/agentctl -run '^TestLiveDockerApplicationProfilesDeliverOTLP$' -count=1 -v` with `AR_IMMS_PR06_APPLICATION_PROFILE_E2E=1`, `AR_IMMS_PR07A_SUPERVISOR_PROFILE_PARITY=1`, verified Collector/Supervisor binary paths, and the isolated test evidence directory.

Result: PASS in 40.48 seconds, exit code 0. Startup fallback and container churn passed. The sink received Redis `redis.clients.connected`, PostgreSQL `postgresql.backends`, Python/Prometheus `python_app_requests_total`, and mock-cooling `ar_imms_test_cooling_celsius`. Each record carried Node ID `33333333-4444-4555-8666-777777777777` and a profile-specific resource ID. The test's cleanup hooks ran; the test log ended with `PASS` and `exit_code=0`.

Pinned evidence: Collector archive SHA-256 `d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf`; Collector executable SHA-256 `43a7d3eb2b759281b6297a91c830988be39c16e538e8f55aeb4ea31ab7a612cd`; Supervisor v0.160.0 SHA-256 `df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449`. Sanitized log SHA-256 `9B4C6F4FE7EE545499ACE0506BEBB25647BC87864519CAED3F8F4DDBF6B3A084`. Local evidence is retained under the ignored `tmp/pr07-win-profile-merged-head-20261114/` directory.

## Implementation and verification

- Added the evidence model and deterministic freshness evaluation contract tests.
- Added a local `doctor` command and expanded concise `status` output; successful collection and all delivery remain unverified without positive evidence.
- Added credential, endpoint-userinfo, and PEM private-key redaction; stripped control characters; and bounded last-error, receiver, endpoint, failure-list, and evidence reason fields.
- Verified sanitized output in the actual `/v1/status` JSON response with credential, bearer token, private key, and endpoint-userinfo fixtures.
- Completeness is component-specific: a timestamp without lifecycle state or configuration generations remains partial, not complete.
- Owner validity deadlines are copied when observations are accepted; an observed zero-generation configuration is complete evidence with an explicit `configuration_not_initialized` reason.
- Inventory and dependency evidence are composed from the existing owner APIs at status snapshot time. Provider failures retain prior inventory and are partial/degraded; dependency inspector diagnostics are reduced to fixed reason codes. No source error string or dependency identity is copied into Health evidence.
- A status endpoint failure retains credential-safe output and now reports a safe category (`timeout`, `dns_resolution_failed`, `connection_refused`, or `request_failed`).
- Preserved the existing `status` Collector state, generation, and pending teardown lines while adding its compact assessment summary. An explicit scrape failure marks collection degraded with unknown freshness; delivery remains unknown.
- Focused health-model and CLI status/doctor tests pass on native Windows.
- Focused verification on the PR-08 worktree passed: health evidence/reporter tests, status/doctor tests, managed Supervisor readiness test, `go vet ./...`, `go build ./...`, architecture guard, and `git diff --check`.
- Exact-head workflow `43534391725` on pre-fix head `4e52c9e9404205c35db8167fd133253a66f03c75` failed on Windows and Ubuntu in the Go `Test` step. Both runners failed the same six `cmd/agentctl` tests: `TestRunHelpWritesCommandSummary`, `TestRunWithoutCommandWritesHelpToStandardError`, `TestRunWithUnknownCommandWritesHelpToStandardError`, `TestRunStatusRendersLiveAgentHealth`, `TestRunStatusReportsUnavailableAgent`, and `TestRunStatusRendersLastError`. Root causes were stale expected help/status output and the compact status renderer dropping previously shown last-error/Collector-endpoint diagnostics. Commit `7a26018` updates those expectations and restores those compact diagnostics; the six focused tests pass locally. CI must rerun on the pushed fix before the gate can advance.
- Markdown relative-link validation passed for all four changed release documents: zero missing targets.
- Full `go test ./... -count=1` remains unverified locally. The non-elevated sandbox hit loopback socket denials, an SQLite `unable to open database file (14)` failure, and Windows Exporter health timeouts; output stopped advancing and the run was interrupted after more than four minutes. The earlier corrected RunAs launch was canceled before tests started. The latest attempt could not start PowerShell through the sandbox and produced no elevated runner log. These results are not a completed product test run. Final native Windows and Linux CI must execute the full suite.
- On 2026-11-14, the prepared elevated runner was checked again. `Start-Process` could not launch the packaged PowerShell path; the native Windows PowerShell launch returned `Access is denied` before UAC. A sandbox-escalated retry produced no runner log or test process, and desktop inspection still showed no UAC prompt. No second elevation result is claimed. The runner remains at `tmp/pr08-windows-full-tests-elevated.ps1`; a genuine High Mandatory Level execution is still required.
- On the post-CLI-fix head `61130fa3af7a6560dbcc07ecdba5b06dfd7a6594`, exact-head CI run [43536412047](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43536412047) passed on Windows and Ubuntu. Both jobs executed the full Go Test step, vet, and build; Ubuntu also ran live Docker/Collector/OTLP profile E2E and managed Supervisor E2E. Windows Docker/profile and native-service lifecycle tests, and Ubuntu native-service upgrade lifecycle E2E, were skipped by workflow conditions. This run predates the current uncommitted review fixes and is not final-head evidence.
- Current-head focused verification on `7f274f3d486a9754e6452702ad969f9eb09e6ff1`: targeted `agenthealth` assessment/reporter tests, `internal/agentlifecycle` tests, owner-evidence adapter tests, bounded SQLite provider-status repository tests, and 13 status/help compatibility tests pass on native Windows. `go vet ./...`, `go build ./...`, and `GOOS=linux GOARCH=amd64 go build ./...` pass. Full health-package tests hit two loopback bind/request denials in the sandbox; no full local Windows suite is claimed. Markdown link validation checked 14 local links with zero missing targets; `git diff --check` passes. The architecture guard invocation failed before import analysis with Go template parsing error `unexpected "," in operand`; the same `go list` template succeeds when invoked directly, so the guard result remains unresolved rather than counted as a pass.
- A fresh UAC prompt is not visible and no elevated process started. Native Linux, final-head Windows/Linux CI, and full regression remain mandatory. Do not treat the earlier successful CI run or skipped E2E jobs as coverage of the current head.

### Review-fix verification — uncommitted

The final review pass moved Inventory and Dependency classification into
`internal/agenthealth`, made stale Inventory explicitly degraded, and made a
Collector receiver failure degrade both Monitoring and Telemetry Collection
while keeping Delivery unknown. Request cancellation now propagates from the
HTTP handler through `Reporter.SnapshotContext`, owner evidence sources, and
the SQLite provider-status query. Tests cover stale snapshots, provider
failure/truncation, bounded dependency projections, Monitoring/Collection/
Delivery separation, request context propagation, and canceled SQLite reads.

Native Windows checks on this working diff: focused `internal/agenthealth`
tests pass; the explicit canceled SQLite provider-status test passes; status/
doctor CLI tests pass; `go vet ./...`, `go build ./...`, and
`GOOS=linux GOARCH=amd64 go build ./...` pass. The full `internal/discovery/sqlite`
tests and `TestIntegrationStatusDoesNotInferHealthOrDelivery` first failed
because the sandbox denied temporary SQLite database creation (`unable to open
database file (14)`). Re-running with Go's `TEMP` and `TMP` redirected into the
writable worktree made both packages pass. A previous wider Health package run
encountered sandbox loopback bind/request denials; exact-head CI must execute
those full tests on Windows and Linux.

A current `go test ./... -count=1` attempt was not completed. It confirmed
sandbox loopback denials in `internal/agenthealth` and `internal/coreapi`, plus
Windows Exporter health timeouts in `internal/dependency`; the run was
interrupted after further packages stopped producing output. Treat this as an
incomplete environment-constrained run, not a full-suite result. Exact-head CI
must execute the complete suite.

The Core/Gateway outage and reconvergence test remains open: no Core health
reporting or Gateway reachability contract is approved, and this change does
not create one or initiate recovery. Current health projections remain local
and source-owned; Delivery stays unknown unless a sink receipt is observed.
Final standards review found no remaining actionable coding-policy issue. The
spec review confirms the stale Inventory and Monitoring/Collection separation
fixes, but retains the Core/Gateway outage-recovery verification gap. PR-08
stays IN_PROGRESS pending the gap disposition and final-head CI.

The current working diff passes `./scripts/check-architecture.ps1`: 36 package
documentation checks, 36 package import checks, and the 450-line production
file guard. This supersedes the earlier pre-fix invocation failure recorded
above. `git diff --check` also passes.

## Core reporting and remaining gates

Core health-report transmission is **not implemented**. It stays open pending an approved contract; local assessment remains usable offline. Remaining PR-08 work: obtain a disposition for the Core/Gateway outage-recovery gate without inventing a contract, run full Windows and Linux test suites on the current exact head, and update this report/tracker after exact-head CI.

**Rollback:** additive assessment data and the `doctor` dispatch can be reverted without changing SQLite state or runtime ownership. Existing `/v1/status` JSON fields and status command invocation remain.

**Next checkpoint:** PR-08 cannot be marked PASS until final-head Windows and Linux verification is green and the remaining review findings are resolved.
