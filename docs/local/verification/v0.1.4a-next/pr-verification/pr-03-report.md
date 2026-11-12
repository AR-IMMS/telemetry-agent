# PR-03 — Agent Service application host and CLI adapter

**Canonical release:** `v0.1.4a-next`<br>
**Status:** VERIFYING — local code gates pass; isolated live Docker E2E and checkpoint CI remain outstanding<br>
**Base:** PR-02 merge `d71f5aa39777824e29f1d06a364bb0a289373803`<br>
**Branch:** `feat/v0.1.4a-next-work`<br>
**Implementation commit:** `3f88d77b68c9d18bec819d3822733fa535052ffc` (`feat(pr-03): add agent application host`)<br>
**Draft PR:** [#22](https://github.com/AR-IMMS/telemetry-agent/pull/22) — `feat/v0.1.4a-next-work` → `chore/v0.1.4a-next`<br>
**CI-tested implementation head:** `0a7c5bd408220655bc1c0b7e54f7b6dc85e1abfb`<br>
**Report date:** 2026-11-12

## Goal and scope

Move long-running Agent Service coordination from `cmd/agentctl` into a small `internal/app` host while preserving the existing CLI adapter, reconciliation controller, discovery provider, proposal/template services, SQLite ownership, schedules, and lifecycle callback behavior. This checkpoint does not migrate domain or SQL ownership, alter persisted state, change CLI contracts, or add OS service-manager behavior.

## Contract and implementation

- `internal/app` now owns the service runtime lifecycle: it composes one bounded desired-state event dispatcher, starts the existing reconciliation loop, optionally runs Docker inventory and proposal/template refresh, and shuts down the dispatcher with a bounded timeout.
- Reconciliation remains the existing controller. Startup and 15-second polling remain the fallback for cross-process writes and missed advisory events; Docker discovery remains on its 30-second cadence.
- `cmd/agentctl/reconciliation_runtime.go` builds the existing controller and services, then passes them through `app.RunRuntime`. The command runner remains an explicit builder input.
- `cmd/agentctl` retains finite command handlers, help/output/exit behavior, enrollment, approvals, install/uninstall, and dependency commands. Repository adapters and domain policy were not copied into `internal/app`.
- `events.Publisher` is now the named contract consumed by Template materialization. SQLite remains the durable authority; the event is a wake-up hint.
- The prior CLI discovery-loop tests were moved into `internal/app`; CLI tests remain in place for finite command and runtime integration behavior.

## Verification

| Check | Environment | Result |
|---|---|---|
| `go test ./internal/app` | Windows worktree | PASS after implementation; runtime-loop cases passed |
| `go test ./internal/app ./cmd/agentctl` | User Windows environment | PASS |
| `go test ./... -count=1` | User Windows environment | PASS; all packages passed. Initial sandbox run failed on SQLite temp-file access (code 14); authorized Windows run passed. |
| `go test -race ./internal/app ./internal/platform/events` | User Windows environment | PASS |
| `go vet ./...` | User Windows environment | PASS |
| `go build ./...` | User Windows environment | PASS |
| `GOOS=linux GOARCH=amd64 go build ./...` | Windows host / Linux target | PASS; cross-build only |
| `scripts/check-architecture.ps1` | Windows source view | PASS; package docs, import boundaries, and production file-size limits passed for 26 packages / 273 production files |
| Fresh PR-03 code review | Read-only independent review | No Critical/Important findings; see noted minor coverage gap below |
| Docker Engine and Compose probe | User Windows environment, elevated access | PASS; Engine 29.5.3 and Compose 5.1.4 reachable |
| `git diff --check` | Worktree | PASS; no whitespace errors |
| Markdown link validation | Six changed local release documents | PASS; 10 local links checked, 0 missing |
| GitHub Actions CI | Draft PR #22, head `0a7c5bd408220655bc1c0b7e54f7b6dc85e1abfb` | PASS; run [43136617157](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43136617157), Ubuntu and Windows passed. This report-only sync triggers a final PR-head rerun. |

### Infrastructure limitations

The live `TestLiveDockerTemplateConvergesThroughAgentRuntime` regression was NOT RUN. Docker is reachable, but the configured live-test variables and disposable Redis container IDs are absent, and no pinned Windows `otelcol-contrib.exe` was found in the standard AR-IMMS install locations or on `PATH`. The running Docker containers belong to the existing MongoDB/observability stacks; they were not changed. Existing documentation records prior live E2E evidence for the pre-refactor composition, but that evidence is not claimed as verification of this PR-03 change.

Native Windows SCM/service lifecycle E2E and native Linux E2E are also NOT RUN. The Linux cross-build does not substitute for Linux execution. Final Windows/Ubuntu CI has not yet run for the PR-03 head.

The independent review found no Critical or Important defect. It noted one minor coverage gap: no isolated unit test exercises `RunRuntime` dispatcher wiring and shutdown as one complete host flow. The gated live Agent runtime test covers that composition path but is NOT RUN here; retain this as a review follow-up alongside the live E2E gate.

## Regression, security, and persistence review

- No migration, table, SQL, enrollment, certificate, secret, Collector artifact, or service-manager behavior changed.
- Dispatcher, discovery, materialization, and reconciliation use the existing shared service context and preserve current safe-code logging semantics.
- The event handler is built before dispatcher startup; the reconciler is validated before lifecycle startup. A failed dependency build returns before the Collector runtime starts.
- The new app-loop tests initially failed to compile against the absent `internal/app` API, then passed after the app host was implemented.
- Existing Docker and observability containers were only listed; no container was created, stopped, or modified.
- Untracked `cmd/agentctl/otel.yaml.lifecycle.lock` remains preserved and is excluded from this checkpoint.

## Changed files

- `internal/app/doc.go`, `internal/app/runtime.go`, `internal/app/runtime_test.go`
- `internal/platform/events/dispatcher.go`
- `cmd/agentctl/reconciliation_runtime.go`, `discovery_runtime.go`, `template_materialization_runtime.go`
- `cmd/agentctl/discovery_runtime_test.go` (moved to `internal/app/runtime_test.go`)
- `cmd/agentctl/template_activation_live_e2e_test.go`, `template_compose_lifecycle_e2e_test.go`, `template_live_e2e_test.go`, `template_materialization_runtime_test.go`
- `docs/local/plans/v0.1.4a-next/pr-03-app-composition.md`
- `docs/local/plans/v0.1.4a-next/release-progress.md`
- `docs/local/verification/v0.1.4a-next/pr-verification/pr-03-report.md`
- PR-01/PR-02 reports and the PR-02 plan receive post-merge status synchronization only.

## Rollback and next checkpoint readiness

Revert PR #22, restore `runRuntimeControlLoops` plus dispatcher lifecycle to `cmd/agentctl`, and retain the PR-02 event publisher contract if its other callers remain; persisted data and schema require no rollback. Keep PR-03 in VERIFYING until final-head CI passes and the live runtime test runs against an isolated Redis fixture with the checksum-verified Collector executable. PR-04 is not ready to start until PR-03 is merged and its context contracts are reviewed.
