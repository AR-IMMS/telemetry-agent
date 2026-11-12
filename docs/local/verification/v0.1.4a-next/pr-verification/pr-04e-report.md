# PR-04e — Reconciliation Runtime Apply Port

**Status:** MERGED — local verification, independent review, and final-head CI PASS<br>
**Release target:** `v0.1.4a-next`<br>
**Branch:** `feat/v0.1.4a-next-work`<br>
**PR / commit:** [#27](https://github.com/AR-IMMS/telemetry-agent/pull/27); implementation `2f1b6301038412bce735d099bbe0a880380298d8`; merge `beb95aa53c6b454c103fecf02b74884811c82483`<br>
**Verification environment:** Windows 11, Go 1.25.5, Docker Engine 29.5.3, Docker Compose 5.1.4

## Implemented

- Added consumer-owned `BaselineReader`, `CandidateApplier`, request/result DTOs under `internal/reconciliation/contracts`, with aliases at the reconciliation package surface.
- Removed `internal/reconciliation`'s direct import of `internal/agentlifecycle` and enforced that direction in `scripts/check-architecture.ps1`.
- Kept candidate compilation, freshness validation, SQLite admission, activation journal, readiness transition, LKG recovery, and runtime process behavior in the existing implementations. The composition root adapts the runtime baseline result to the consumer snapshot contract.
- Retained `agentlifecycle.CleanBaselineSnapshot` as the lifecycle baseline builder's return type; the composition adapter maps its fields into the reconciliation-owned snapshot. Left `internal/agenthealth` in place after confirming it already owns transient health evaluation and local status adapters.
- No SQLite schema/migration, CLI output, Core authority, OpAMP/security policy, Collector configuration, dependency lifecycle, or health behavior changed.

## Contract and architecture rulings

- Reconciliation owns the consumer ports and DTOs; Runtime owns the application behavior. This follows the documented consumer-owned-port rule and removes reconciliation's concrete dependency on lifecycle code.
- Contract definitions live in the leaf `internal/reconciliation/contracts` package, with aliases in the parent package. Importing the parent from runtime created a test-time import cycle because same-package reconciliation tests use runtime fixtures; the leaf package preserves ownership and avoids changing tests to an external package.
- `collectorruntime.Candidate` remains the immutable compiled candidate value. It is not duplicated in another DTO.
- Baseline snapshots carry the observed generation/fingerprint but do not claim a cross-store read transaction; runtime admission rechecks freshness.
- Health code was not moved because the current `agenthealth` package has no demonstrated cross-context persistence dependency.

## Tests and verification

| Test / check | Environment | Result |
|---|---|---|
| `go test ./internal/reconciliation/... ./internal/agentlifecycle ./cmd/agentctl -count=1` | Windows, normal filesystem access | PASS |
| `go test ./... -count=1` | Windows, normal filesystem access | PASS |
| `go test -race ./internal/reconciliation/... ./internal/agentlifecycle ./cmd/agentctl -count=1` | Windows, normal filesystem access | PASS on full rerun. One earlier concurrent run hit the existing 1-second startup assertion; the isolated test and complete rerun passed. |
| `go vet ./...` | Windows | PASS |
| `go build ./...` | Windows | PASS |
| `GOOS=linux GOARCH=amd64 go build ./...` | Windows cross-build | PASS; this is not native Linux execution. |
| `scripts/check-architecture.ps1` | Windows | PASS; 31 packages, 286 production Go files. The new rule failed against the pre-refactor import and passed after the inversion. |
| `gofmt` on changed Go files | Windows | PASS |
| Live `TestLiveDockerTemplateConvergesThroughAgentRuntime` | Windows, three disposable Redis 7 containers, pinned `otelcol-contrib` 0.160.0 | PASS; complete lifecycle and telemetry E2E, 95.79s. The Collector archive was verified against the repository-pinned SHA-256 and size before use. |
| `git diff --check` and Markdown relative links | Windows | PASS |
| PR #27 CI run `43178655519` | GitHub Actions, `ubuntu-latest` and `windows-latest` | PASS; tests, vet, build, Collector capability probe, and architecture checks passed on both platforms. |

Independent review found that the initial import guard only matched the root reconciliation package; it now covers reconciliation subpackages as well. Review also corrected the documentation to describe `CleanBaselineSnapshot` as the lifecycle baseline builder's return type, not as a current CLI-consumed contract. The updated diff has no remaining actionable review findings.

The first live-E2E attempt supplied Docker's abbreviated container IDs and failed its discovery precondition before runtime activation. The rerun used full IDs, passed, and verified actual Collector startup, receiver activation, OTLP sink traffic, restart, disable, and delete behavior. The three task-created Redis containers and downloaded Collector test artifact were removed after the run.

## E2E and limitations

- Windows live Collector/Docker runtime E2E: PASS for the covered Template-to-runtime path. The test does not install the Windows SCM service.
- Linux/amd64: cross-build PASS; native Linux runtime/E2E NOT RUN.
- PR #27 merged to `chore/v0.1.4a-next` at `beb95aa53c6b454c103fecf02b74884811c82483` after both platform CI jobs passed.
- The unrelated untracked `cmd/agentctl/otel.yaml.lifecycle.lock` was preserved and excluded from all changes.
- `docs/shared/` was not modified.

## Rollback and next checkpoint

This change moves interface and DTO ownership only; it adds no persistent data. Before merge, revert the PR-04e implementation commit. After merge, revert the merge commit with its first parent retained. The next PR-04 slice is evidence-gated platform dependency adapter extraction; do not change installation ownership, checksums, retry, or teardown semantics.
