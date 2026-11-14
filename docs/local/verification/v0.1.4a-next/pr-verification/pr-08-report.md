# PR-08 Health & Reporting Verification

- **Status:** VERIFIED FOR SUPPORTED PR-08 SCOPE; PR #31 remains draft and unmerged.
- **Branch:** `feat/pr-08-health-reporting`
- **Base:** PR-07 merge `700768dff651e73a2e457aa20d94845bc326d971`
- **Latest PR head:** `ea4a70189cf57a45b151080c6dfe82fd4cdc945d`, pushed to the existing PR branch. This verification-record commit follows implementation head `f528f9b0e1066f7137afc908fd141dfc0bb0afb7`.
- **Implementation commits:** prior PR-08 commits plus `1ce2106f4884750cea83805fe5b564677c328cd3` (Core/OpAMP/Gateway evidence and exporter diagnostics) and `f528f9b0e1066f7137afc908fd141dfc0bb0afb7` (CI assertion correction and verification update).
- **PR:** draft [#31](https://github.com/AR-IMMS/telemetry-agent/pull/31), targeting `chore/v0.1.4a-next`.

## Goal and boundaries

Provide truthful local Agent health evidence and diagnostics. Health projects evidence owned by Runtime, Inventory, Monitoring, Configuration, dependencies, and supported connectivity adapters; it does not start/restart processes, mutate config, or claim backend delivery from Collector readiness or OpAMP state. Core health-report transmission remains unimplemented until an approved versioned Core contract defines authentication, status vocabulary, retries, idempotency, retention, and replay.

The source tree already had a local `/v1/status` API, a `status` CLI command, and an `internal/agenthealth` package. PR-08 extends those components instead of adding another reporting pipeline.

## Source-checked evidence matrix

| Dimension | Current owner/source | Available evidence | Freshness and completeness | PR-08 projection |
|---|---|---|---|---|
| Agent runtime | Local status HTTP handler | A successful response proves the Agent status endpoint handled a request | Generated for the current response | `healthy`, response timestamp |
| Core connectivity | Existing authenticated Core API communication, where an Agent-owned request result is available | Current enrollment is an explicit CLI operation and does not prove continuing availability; the running Agent currently exposes no authenticated request result to Health | No Agent-owned observation or freshness deadline is available today | Use existing request outcomes when wired: classify network failure, authentication failure, and unavailable/stale evidence separately; otherwise `unknown: core_connectivity_evidence_unavailable` |
| OpAMP connectivity | Existing Supervisor/OpAMP connection and session evidence | Supervisor process state is available; the remote WSS session owner does not currently expose connection/session events to Agent Health | No local connected/disconnected event or validity deadline is available today | Use session evidence independently of Core availability when exposed; otherwise `unknown: opamp_session_evidence_unavailable`; Supervisor process health remains separate |
| Gateway connectivity | Supported Collector exporter diagnostics and telemetry evidence | Legacy Collector output callback recognizes structured exporter network failures; managed Supervisor does not expose child Collector diagnostics to the Agent | Failure timestamp is recorded without an owner validity deadline; absence of failure is not success | Reuse observed exporter diagnostics where available; no custom probe. A failure marks Gateway connectivity degraded with unknown freshness; otherwise unknown. Readiness does not prove OTLP receipt |
| Collector | Runtime watcher and Collector readiness probe | Lifecycle state, desired/activated/applied generations, pending teardown count; sanitized structured scrape-failure observations | Collector/Supervisor completeness requires an observed lifecycle state; freshness needs owner-provided `validUntil` | Healthy/degraded/unknown state, component-specific completeness and explicit freshness |
| Configuration | Configuration/Runtime state | Generation equality and pending teardowns; applied means generation-bound runtime readiness | Completeness requires the runtime state/generation observation; timestamp alone is partial; no implicit age policy | Healthy only for equal nonzero generations with no pending teardown; otherwise degraded/unknown |
| Inventory | Inventory SQLite repository | Provider enabled state, last attempt/success, error code, and resource count; Health reads at most 64 provider rows and never loads the resource inventory | Uses provider timestamps and Discovery's `DefaultPolicy().StaleAfter` (two minutes); provider error makes the projection partial; a truncated read remains unknown/partial | Latest provider failure is `degraded`; all enabled providers with successful snapshots are `healthy`; missing evidence is `unknown`; stale last success is marked stale |
| Monitoring | Monitoring profile and endpoint diagnostics | Profile match, endpoint resolution, and timestamped Collector scrape failure; no success counter proving current collection | Failure evidence has source timestamp; no success inference from absence of failures | An observed scrape failure marks collection degraded with unknown freshness; no failure and no success evidence remains unknown |
| Dependencies | Dependency status adapters | Read-only availability, health, drift, and per-inspector failure projection from the existing `dependency status` service | Queried for each health snapshot with a five-second overall context deadline; freshness is request-scoped | Enabled dependencies all healthy is `healthy`; unhealthy/drifted or failed inspection is `degraded`; incomplete evidence is `unknown`; no enabled dependencies is `unsupported` |
| Telemetry collection | OpenTelemetry Collector | A matching receiver failure can prove a collection attempt failed; readiness or lack of failure cannot prove scraping | Failure is timestamped; no verified-success freshness source is available | `degraded` with unknown freshness on explicit failure; otherwise `unknown`; successful collection remains unverified |
| Telemetry delivery | Collector/export path | Actual sink receipt is verified only by isolated E2E fixtures; Agent runtime has no production receipt signal | No production freshness/completeness source | `unknown: backend_receipt_not_measured` |

### Contract and conflict findings

- Existing top-level `Snapshot.Status` reflects Collector readiness, applied configuration generations, pending teardowns, and the last runtime error. It is not a full system health result. Existing keys remain for compatibility; the new assessment is additive. A nil reporter retains the existing degraded top-level status while its detailed evidence is unknown.
- Existing Collector network failures are reduced to direction, receiver or exporter ID, host/port, code, and observation time; raw Collector log lines are not retained. The list is bounded at 32, but previously accepted identifier strings had no length limit and `LastError` was exposed verbatim. PR-08 bounds/redacts those diagnostics. Exporter failures are evidence of a failed send attempt only. The Collector also exposes exporter send-failure internal telemetry when configured; this Agent does not claim receipt from Collector readiness or missing errors. See the [Collector exporter internal telemetry reference](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/documentation.md).
- Readiness and a successful config-generation application are not proof of scrape or delivery. `doctor` shows those as separate dimensions; explicit scrape failures mark collection degraded, while absence of both success and failure evidence remains unknown. Delivery remains unknown without sink receipt.
- Health consumes Inventory and Dependency projections through read-only sources composed at the `agentctl` boundary. Inventory reads only provider-status rows through Discovery's capped `ProviderStatusReader` (64 maximum) and uses the Discovery-owned two-minute stale policy. Dependency inspection is bounded by a five-second request deadline; neither adapter mutates owner state or starts a separate polling loop.
- The HTTP request context now reaches both owner evidence sources. Inventory provider-status reads accept that context and SQLite checks cancellation before opening the database and uses `QueryContext`; Dependency derives its bounded timeout from the same caller context. The inventory read remains capped at 64 rows.
- A failed Agent-state read now degrades the Agent runtime dimension while leaving Collector, Supervisor ownership, and configuration unknown. An observed Collector failure remains degraded. This avoids turning an Agent database/read error into a fabricated Collector failure or a false legacy-ownership result.
- No approved Core reporting endpoint or wire contract is present in the local contracts. No Core route, persistence model, retry policy, or status mapping is invented.
- Core connectivity reuses existing authenticated Core API request outcomes where available; no new request or probe is introduced. In this source revision the running Agent has no such request result wired into Health, so Core remains unknown. Network failure, authentication failure, and unavailable/stale evidence are distinct classifications.
- OpAMP connectivity reuses existing Supervisor/OpAMP connection and session evidence where exposed. OpAMP connectivity never stands in for Core availability. No session signal currently reaches Agent Health, so OpAMP connectivity remains unknown while Supervisor process health is assessed separately.
- Gateway health reuses supported Collector exporter diagnostics and telemetry evidence. The legacy output callback can surface structured exporter failures, but the managed Supervisor path does not expose child Collector diagnostics. No custom Gateway probe is added; readiness and lack of errors do not imply delivery.
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
the SQLite provider-status query. The current follow-up also adds separate
OpAMP and Gateway connectivity dimensions, keeps OpAMP session evidence
unknown unless reported, and classifies Collector exporter network failures
without treating Collector readiness or missing errors as delivery proof.
Tests cover stale snapshots, provider failure/truncation, bounded dependency
projections, Monitoring/Collection/Delivery separation, request context
propagation, canceled SQLite reads, unknown OpAMP session evidence, and an
exporter network failure while Collector readiness remains healthy.

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

Core reporting and Core/OpAMP outage reconvergence are deferred because the
Agent lacks an existing current Core request source and the remote OpAMP
session owner does not expose its session state to this process. These
contract-dependent gates do not block local PR-08 completion. Gateway failure
evidence is read from Collector output only where the Agent owns that output
callback; managed Supervisor child Collector output is not exposed to the
Agent and remains unknown without another supported evidence source. Delivery
remains unknown unless a sink receipt is observed. Final standards review
found no remaining actionable coding-policy issue. The spec review confirms
the stale Inventory and Monitoring/Collection separation fixes; contract-
dependent scenarios remain `NOT_TESTED`, not passed.

The current working diff passes `./scripts/check-architecture.ps1`: 36 package
documentation checks, 36 package import checks, and the 450-line production
file guard. This supersedes the earlier pre-fix invocation failure recorded
above. `git diff --check` also passes.

### Exact-head CI — 2026-11-14

PR head `5f9336ec944d9ecaa78e64d36c6c24334e08a649` passed Actions run
[43577440528](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43577440528)
on Windows and Ubuntu. Both runners executed and passed the full `go test
./... -count=1` step, architecture checks, pinned Supervisor verification,
managed Supervisor fallback/OTLP E2E, vet, and build. Ubuntu also passed the
isolated Docker/Collector/OTLP profile E2Es, including Redis, PostgreSQL,
Python/Prometheus, mock-cooling, and container churn. Windows Docker/profile
E2Es were skipped by workflow conditions; native service lifecycle E2E was
skipped on both jobs because this run was for PR #31. These skipped cases are
not counted as covered by this run.

Sanitized artifacts were downloaded and hashed under the ignored local path
`tmp/pr08-ci-43577440528/`:

- Windows managed enrollment/OpAMP: artifact `14107151884`, file SHA-256
  `B328B3CFFAFBC3D125E95D7AC287D4060C8D9DE56B4EFF18F7E6873A1C6B7E86`.
- Windows managed Supervisor runtime: artifact `14107151884`, file SHA-256
  `694D05DCB4EC36C73191B6FB93A0DA86D2DC6EA7F91702832802287A648B4F65`.
- Ubuntu managed enrollment/OpAMP: artifact `14107661061`, file SHA-256
  `E11E69F71C726A2A04A061032E24759A4CF9CDE3D3919B4D89A4DBED35F22721`.
- Ubuntu managed Supervisor runtime: artifact `14107661061`, file SHA-256
  `63D636BFF358408056AD3B54345429E303C1FEAF4F97CB788EB4BAD516E70A73`.
- Ubuntu profile summary and OTLP sink: artifact `14107431384`, file SHA-256s
  `A6E21606A94598CA151D08051E40DADE2E598C0219A1DC9B82AD7A07C1CDA47B` and
  `CAA41CC09E09954BBDA3CE4540062F4AF252E959DF03A42334A7909F6C1F2849`.

## Follow-up verification on pushed commit

Commit `1ce2106f4884750cea83805fe5b564677c328cd3` added separate OpAMP and
Gateway connectivity evidence and Collector exporter failure classification.
Exact-head CI run [43580157943](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43580157943)
failed on both Windows and Ubuntu in the `Test` step. The sole failure was
`TestRunDoctorExplainsUnknownAndStaleEvidence`, whose expected output still
used `core_connectivity_not_observed` after the reason code changed to
`core_connectivity_evidence_unavailable`. The architecture check passed on
both runners. Later tests, vet, and build were skipped by workflow dependency
because `Test` failed; Ubuntu's isolated Docker setup succeeded, but live
profile tests did not run.

Commit `f528f9b0e1066f7137afc908fd141dfc0bb0afb7` updates that stale test
assertion in `cmd/agentctl/status_command_test.go`; the corrected focused test
passes locally. Exact-head CI run
[43580653489](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43580653489)
passed on Windows and Ubuntu. Both jobs passed the full Go test step,
architecture checks, managed Supervisor fallback/OTLP E2E, vet, and build.
Ubuntu also passed live Docker/Collector/OTLP and Docker application profile
E2Es. Windows Docker/profile E2Es and native service lifecycle E2Es on both
OSes were skipped by workflow conditions and are not counted as covered.

- Passed focused Windows tests for unknown Core/OpAMP evidence, exporter
  network-failure classification, sanitized Collector failures, and the
  legacy runtime output callback forwarding exporter diagnostics.
- Passed the full `internal/agentlifecycle` package after setting Go `TEMP` and
  `TMP` inside the writable worktree.
- After correcting the stale CLI expectation, focused
  `TestRunDoctorExplainsUnknownAndStaleEvidence` passed locally.
- `go vet ./...`, native `go build ./...`, and
  `GOOS=linux GOARCH=amd64 go build ./...` passed.
- `./scripts/check-architecture.ps1` passed package documentation checks for
  36 source directories, import-boundary checks for 36 packages, and the
  production Go file-size guard. `git diff --check` passed.
- Changed-document link validation checked 20 relative links across five
  changed Markdown files; zero targets were missing.
- The full `internal/agenthealth` package did not pass locally: two existing
  HTTP tests (`TestFetchStatusDecodesLiveSnapshot` and
  `TestServeStatusServesSnapshotAndStopsWithContext`) were blocked by Windows
  sandbox loopback permissions (`connectex` access denied). The focused tests
  passed; this environment failure is not treated as a product regression.
- The CI artifact records for run `43580653489` are retained by GitHub through
  2027-02-12: PR-07a Supervisor/profile parity artifact `14108696584`
  (SHA-256 `b462a28e0410402cfc0d0c11a4e8c3209f15bee63b2974897d14413826ed8705`),
  Linux managed Supervisor artifact `14108127959` (SHA-256
  `6fc311eb06971498db0346f13baa73e5461a6ea9d5d207d96992b51fd378c89b`), and
  Windows managed Supervisor artifact `14108127691` (SHA-256
  `93bddf91191c5fa6d2d605c1dd77dd4b122d4ab08b0bf37276bd3e21a7e46d71`).
- Commits `1ce2106` and `f528f9b` were pushed to
  `origin/feat/pr-08-health-reporting`. The connected GitHub read API confirms
  PR #31 remains open and draft at corrected head `f528f9b`; exact-head run
  `43580653489` passed. The GitHub connector denied PR-body updates with HTTP 403
  (`Resource not accessible by integration`); direct `gh` API calls failed
  with a network error, so the PR description is unchanged.

## Contract disposition and remaining gates

Per the approved PR-08 clarification, external Core health-report transmission is **deferred** until a versioned Core contract is approved. This does not block independent local health work. Core network failure, authentication failure, and unavailable/stale evidence remain distinct; the running Agent currently has no authenticated Core-request result source wired into Health, so this dimension is `UNKNOWN`. OpAMP session evidence is independent and is also `UNKNOWN` until the local Agent receives the Supervisor's existing session evidence. Gateway failure may be classified from supported Collector exporter diagnostics where those diagnostics reach the Agent; Collector readiness or absence of errors does not prove reachability or OTLP receipt. No custom Gateway probe, Core endpoint, or reporting schema is introduced.

Outage and recovery cases must use existing owner evidence. The legacy direct Collector output callback can report an observed exporter network failure, while the managed Supervisor path does not currently expose child Collector output to Agent Health. Tests cover classification and recovery boundaries where evidence is available; Core reporting and cross-process Core/OpAMP reconvergence are recorded `BLOCKED`/`NOT_TESTED` with the missing source/contract and follow-up, and do not block PR-08 as a whole. Local snapshots and read-only reporting ports remain available offline.

The supported PR-08 implementation and verification gates pass. Exact PR head `ea4a70189cf57a45b151080c6dfe82fd4cdc945d` passed Windows and Ubuntu CI run [43581426398](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43581426398). Both jobs passed full Go tests, architecture checks, managed Supervisor fallback/OTLP E2E, vet, and build. Ubuntu also passed live Docker/Collector/OTLP and Docker profile E2Es. Hosted Windows Docker/profile and native service lifecycle E2Es on both OSes were skipped and remain unverified.

External Core report transmission remains `BLOCKED` for release integration until its versioned contract is approved. Core/OpAMP reconvergence and managed Supervisor child Collector diagnostics remain `NOT_TESTED` where their existing evidence sources are unavailable. Per the user's approved disposition, these cases do not block PR-08 as a whole. PR #31 has not been marked ready or merged. Its description is stale because the connected GitHub integration denied body writes (HTTP 403), and direct `gh` API calls cannot reach GitHub; updating PR metadata or merging needs a write-capable path.

**Rollback:** additive assessment data and the `doctor` dispatch can be reverted without changing SQLite state or runtime ownership. Existing `/v1/status` JSON fields and status command invocation remain.

**Next checkpoint:** PR-08 is verified for its supported scope. The PR description still needs synchronization through a write-capable GitHub path. Once updated, mark the PR ready and follow the integration-branch merge workflow. Do not treat the deferred contract-dependent cases as PR-08 blockers; track them for final release acceptance.
