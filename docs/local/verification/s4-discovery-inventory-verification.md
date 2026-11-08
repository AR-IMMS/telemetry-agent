# S4 Discovery & Inventory Verification

## Verification matrix

| Area | Command / evidence | Result | Notes |
|---|---|---|---|
| Discovery domain and Docker adapter | `go test ./internal/discovery/... -count=1` | PASS | Snapshot completeness, validation, identity, normalization, state transitions, secret filtering, endpoint evidence, and provider failures. |
| SQLite inventory | `go test ./internal/agentstate -run '^TestSQLiteInventory' -count=1 -v -timeout=60s` | PASS | Populated v9 migration, M1/M2 and enrollment/S1 preservation, CAS, no-op timestamps, rollback, concurrent updates, consistent snapshots, and restart. |
| CLI and runtime wiring | `go test ./cmd/agentctl -run '^(TestInventory|TestRuntimeControlLoops|TestRunHelp)' -count=1 -timeout=120s` | PASS | Read-only list/inspect/status; discovery loop startup/cancellation; root command help. |
| Full Windows suite | `go test ./... -count=1` | PASS | Native Windows, all repository packages. Opt-in live tests skip unless explicitly enabled. |
| Vet | `go vet ./...` | PASS | Windows. |
| Native Windows build | `go build -buildvcs=false ./...` | PASS | Windows/amd64. |
| Linux build compatibility | `$env:GOOS='linux'; $env:GOARCH='amd64'; go build -buildvcs=false ./...` | PASS | Cross-build only; does not prove native Linux runtime. |
| Race tests | `go test -race ./internal/discovery/... ./internal/agentstate ./cmd/agentctl` | PASS | Windows/amd64; all four packages. |
| Live Docker inventory E2E | `TestLiveDockerInventoryWhenExplicitlyEnabled` using Redis fixture `ar-imms-s4-e2e-20261109-01` | PASS | Windows named pipe → Moby client → coordinator → disposable SQLite → read-only CLI. Restart preserved full container ID; stop showed `exited`; removal became `missing` after two successful misses. Repeated stable snapshots did not advance material revision. |
| Live Docker failure isolation | `TestLiveDockerUnavailableRetainsInventory` against an unreachable local named pipe | PASS | Persisted safe `daemon_unavailable`; resource rows/revision and M2 generations remained unchanged. |
| Actual Agent Service + Collector lifecycle | Temporary pinned Collector 0.160.0 bootstrap followed by `agentctl run`, first with Docker discovery enabled, then disabled | BLOCKED | Both runs reached Collector startup and then failed identically during the existing S3 candidate-triggered Windows restart: `stop Collector before configuration restart`, exit `0xc000013a`. The same failure with discovery disabled isolates it from S4; no S3/M2 changes are included here. This prevents claiming full Agent Service E2E or continued Collector operation across the restart. |
| Linux native runtime / Docker E2E | Linux host runner | NOT RUN | No native Linux Docker host in this session. CI Linux test job remains required. |
| GitHub Actions | Draft PR workflow | NOT RUN | To be checked after PR creation. |

## Live Docker fixture procedure

The live runs used unique labeled Redis containers and a disposable SQLite file under the current user's temporary directory. The fixture was restarted, stopped, removed and observed across separate test processes. Failure testing used a nonexistent local named pipe. Cleanup removed only the two S4-owned fixture names; no production Agent database was used.

The test verifies Integration desired revision and lifecycle generations do not change during discovery. It does not measure OTLP delivery. Collector startup/readiness logs were observed during the separate full-service attempt, but the restart failure means no end-to-end claim of sustained Collector operation is made.

## Remaining verification limits

- Cross-compilation is not equivalent to native Linux runtime verification.
- Full Agent Service operation remains blocked by the reproduced Windows S3/M2 candidate restart failure.
- Collector readiness is not proof of telemetry delivery.
- Docker Engine permission behavior on a native Linux service account remains for Linux CI/operator verification.
- Integration matching, proposal/approval, Docker events and non-Docker providers remain deferred.
