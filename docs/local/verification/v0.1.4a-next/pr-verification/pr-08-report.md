# PR-08 Health & Reporting Verification

- **Status:** IN_PROGRESS
- **Branch:** `feat/pr-08-health-reporting`
- **Base:** PR-07 merge `700768dff651e73a2e457aa20d94845bc326d971`
- **Implementation commits:** `b634eed40ae7f7b6996384f1370c7ec1994e31ab`, `13398ced1c1d16de992025f8927636d3bc3695e2`, `082f7535047f393108690d0c64c0caf31b76efa4`, `4ab5958b344cd02330b04142a7ef53cad3b4b667`
- **PR:** not opened yet.

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
| Inventory | Inventory SQLite repository | Provider enabled state, last attempt/success, error code, completeness and per-resource presence; existing inventory freshness policy is two minutes | Provider timestamps and explicit `DefaultPolicy().StaleAfter`; completeness belongs to provider snapshot | Not yet aggregated into health; explicit `unknown` |
| Monitoring | Monitoring profile and endpoint diagnostics | Profile match, endpoint resolution, and timestamped Collector scrape failure; no success counter proving current collection | Failure evidence has source timestamp; no success inference from absence of failures | An observed scrape failure marks collection degraded with unknown freshness; no failure and no success evidence remains unknown |
| Dependencies | Dependency status adapters | Read-only availability, health, and drift projection in `dependency status` | Queried at command time; not part of the local health snapshot | Not yet aggregated into health; explicit `unknown` |
| Telemetry collection | OpenTelemetry Collector | A matching receiver failure can prove a collection attempt failed; readiness or lack of failure cannot prove scraping | Failure is timestamped; no verified-success freshness source is available | `degraded` with unknown freshness on explicit failure; otherwise `unknown`; successful collection remains unverified |
| Telemetry delivery | Collector/export path | Actual sink receipt is verified only by isolated E2E fixtures; Agent runtime has no production receipt signal | No production freshness/completeness source | `unknown: backend_receipt_not_measured` |

### Contract and conflict findings

- Existing top-level `Snapshot.Status` reflects Collector readiness, applied configuration generations, pending teardowns, and the last runtime error. It is not a full system health result. Existing keys remain for compatibility; the new assessment is additive. A nil reporter retains the existing degraded top-level status while its detailed evidence is unknown.
- Existing Collector network failures are reduced to receiver, host/port, code, and observation time; raw Collector log lines are not retained. The list is bounded at 32, but previously accepted identifier strings had no length limit and `LastError` was exposed verbatim. PR-08 bounds/redacts those diagnostics.
- Readiness and a successful config-generation application are not proof of scrape or delivery. `doctor` shows those as separate dimensions; explicit scrape failures mark collection degraded, while absence of both success and failure evidence remains unknown. Delivery remains unknown without sink receipt.
- `inventory status` and `dependency status` already expose owner-produced evidence, but there is no health-owned aggregation adapter. PR-08 reports these dimensions as unknown instead of duplicating their collectors or inspectors.
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
- Preserved the existing `status` Collector state, generation, and pending teardown lines while adding its compact assessment summary. An explicit scrape failure marks collection degraded with unknown freshness; delivery remains unknown.
- Focused health-model and CLI status/doctor tests pass on native Windows.
- Focused verification on the PR-08 worktree passed: health evidence/reporter tests, status/doctor tests, managed Supervisor readiness test, `go vet ./...`, `go build ./...`, architecture guard, and `git diff --check`.
- Markdown relative-link validation passed for all four changed release documents: zero missing targets.
- Full `go test ./... -count=1` remains unverified locally. The non-elevated sandbox hit loopback socket denials in HTTP tests. A RunAs probe verified High Mandatory Level and an enabled Administrators SID, but that first runner had a repository path error before tests started. The corrected RunAs launch was canceled by Windows with `The operation was canceled by the user`; its test log was not produced. This is not a product test failure. Final native Windows CI must execute the full suite.
- On 2026-11-14, the prepared elevated runner was checked again. `Start-Process` could not launch the packaged PowerShell path; the native Windows PowerShell launch returned `Access is denied` before UAC. A sandbox-escalated retry produced no runner log or test process, and desktop inspection still showed no UAC prompt. No second elevation result is claimed. The runner remains at `tmp/pr08-windows-full-tests-elevated.ps1`; a genuine High Mandatory Level execution is still required.
- Native Linux, full regression, race, and final-head Windows/Linux CI are still required. The architecture guard, vet, and build have passed locally.

## Core reporting and remaining gates

Core health-report transmission is **not implemented**. It stays open pending an approved contract; local assessment remains usable offline. Remaining PR-08 work: complete code/security review, run full Windows and Linux test suites, verify failure/recovery projections and no delivery inference on final head, and update this report/tracker to the final PR head.

**Rollback:** additive assessment data and the `doctor` dispatch can be reverted without changing SQLite state or runtime ownership. Existing `/v1/status` JSON fields and status command invocation remain.

**Next checkpoint:** PR-08 cannot be marked PASS until final-head Windows and Linux verification is green and the remaining review findings are resolved.
