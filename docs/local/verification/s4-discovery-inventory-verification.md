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
| Actual Agent Service + Collector lifecycle | Isolated temporary state/configuration, pinned Collector 0.160.0, `agentctl run`, and explicit local Windows Docker named pipe | PASS | The Agent stayed running through the candidate-triggered Collector replacement. Integration status reported desired revision 0 as Applied at generation 1; inventory status reported Docker enabled with 22 resources and no provider error. Collector logs showed the old process handling CTRL_BREAK and completing shutdown, then a new process becoming ready. Temporary Agent/Collector processes were stopped after verification. This proves lifecycle continuity and readiness, not OTLP delivery. |
| Linux native runtime / Docker E2E | Linux host runner | NOT RUN | No native Linux Docker host in this session. Ubuntu CI covers compilation/tests, not native Docker runtime behavior. |
| GitHub Actions | [Run 42536179870](https://github.com/AR-IMMS/telemetry-agent/actions/runs/42536179870), head `e770035a473474da095e1b77c7b963a3fda506d5` | PASS | `Verify (ubuntu-latest)` and `Verify (windows-latest)` completed successfully. |

## Live Docker fixture procedure

The live inventory runs used unique labeled Redis containers and a disposable SQLite file under the current user's temporary directory. The fixture was restarted, stopped, removed and observed across separate test processes. Failure testing used a nonexistent local named pipe. Cleanup removed only the S4-owned fixture names; no production Agent database was used. The full Agent runtime check used a separate temporary root and localhost-only Gateway endpoint; it did not use a production Agent database or remote Gateway.

The live inventory test verifies Integration desired revision and lifecycle generations do not change during discovery. The full Agent runtime check verifies the service remained active while the existing lifecycle replaced the Collector. Neither check measures OTLP delivery.

## Remaining verification limits

- Cross-compilation is not equivalent to native Linux runtime verification.
- Native Linux Agent + Docker runtime and service-account permission behavior remain unverified in this session.
- Collector readiness is not proof of telemetry delivery.
- The Windows Agent runtime check discovered the Docker containers visible to the current local Docker account; it did not prove telemetry delivery to a Gateway.
- Integration matching, proposal/approval, Docker events and non-Docker providers remain deferred.
