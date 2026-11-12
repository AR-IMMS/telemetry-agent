# PR-01 — verified source and CLI baseline

**Baseline commit:** `1a32602bd1d655c1bdfbf8d477f55b42ac27b49a`<br>
**Platform:** Windows workspace, amd64<br>
**Purpose:** capture compatibility evidence before PR-01 source changes. This records observed behavior only; it does not define new CLI behavior.

## Root help observation

Command: `go run ./cmd/agentctl help`<br>
Exit code: `0`

```text
Usage:
  agentctl <command> [options]

Commands:
  bootstrap   Bootstrap the OpenTelemetry Collector.
  dependency  Manage telemetry dependencies.
  integration Manage desired Integration instances and local convergence diagnostics.
  run         Run the OpenTelemetry Collector under supervision.
  inventory   Read persisted resource discovery inventory.
  discovery   Review and approve durable monitoring proposals.
  status      Show live Agent health.
  enroll      Bind this Agent installation to an AR-IMMS Core node.
  install     Install and start the Agent as a managed service.
  uninstall   Safely remove the Agent and its managed dependencies.
  help        Show this help.
```

Observed compatibility notes:

- `go run ./cmd/agentctl --help` exits `2`, reports `unknown command "--help"`, then prints the same command list. Do not infer a supported `--help` alias.
- The source routes a `service` command, and Windows implementation has `agentctl service --state-path <path>`, but root help omits it. The target CLI contract labels this Windows/internal. PR-01 records the discrepancy; any help behavior change belongs to the dependent CLI checkpoint.
- Existing commands use `--state-path`. Source tests exercise `--expected-revision`, `--expected-desired-revision`, and integration/template `--output json`; exact applicability remains command-specific.
- Existing CLI tests cover persisted inventory, integration/template lifecycle and revision conflicts, bootstrap, managed dependencies, and installation. Preserve these workflows; do not replace them with proposals in the vNext contract.

## Source baseline observations

- Composition and command routing currently live in `cmd/agentctl`; existing context packages include `agentstate`, `configuration`, `reconciliation`, `agentlifecycle`, `agentinstallation`, `collectorruntime`, `discovery`, `integration`, `identity`, `enrollment`, `agenthealth`, `dependency`, `coreapi`, and `coreconfig`.
- `internal/collectorruntime/manifest.go` declares `otelcol-contrib` artifact manifest `0.160.0` and validates archive SHA256 values. Existing receiver catalog includes OTLP, Prometheus, and Redis.
- `internal/agentstate` has a SQLite migration ledger; source tests assert schema migration version 13. PR-01 must not add or alter a migration.
- Existing tests cover SQLite identity/inventory/Integration/template state, candidate activation and recovery, reconciliation, Docker inventory snapshots, and Collector artifact metadata. Their existence is not evidence of full OS/infrastructure E2E. The production-file scan measured `internal/enrollment/service.go` at 454 lines, so PR-01 uses an exact 454-line transition ceiling for that existing file; the guard rejects any further growth.

## Verification commands and results

| Command | Result | Environment detail |
|---|---|---|
| `go test ./...` | PASS | All packages passed on Windows with normal temp-file access. The unprivileged sandbox run had filesystem/SQLite access-denied errors, so it was not used as the baseline result. |
| `go vet ./...` | PASS | Windows workspace. |
| `go build ./...` | PASS | Windows workspace. |
| `$env:GOOS='linux'; $env:GOARCH='amd64'; go build ./...` | PASS | Cross-compilation only; not native Linux verification. |
| `docker version --format 'Client={{.Client.Version}} Server={{.Server.Version}}'` | PARTIAL | Normal sandbox access is denied. Elevated read-only access confirmed Client and Engine 29.5.3 and Compose 5.1.4; the daemon is Linux on Docker Desktop. Existing Core and observability stacks are running, so use an isolated fixture before infrastructure tests. |
| Native Linux E2E | NOT RUN | No native Linux host was available in this verification. |
| GitHub CLI/API | BLOCKED | `gh` 2.96.0 installed; `GH_TOKEN` and saved auth are invalid; API connection failed. No remote changes were made. |
| Shared coding policy | BLOCKED | `docs/shared/engineering/AI_CODING_POLICY.md` could not be read because the shared submodule is inaccessible/unavailable in this checkout. |

These checks establish the local source baseline only. They do not satisfy release acceptance for native Windows/Linux service lifecycle, Docker, real Collector telemetry, Core enrollment or gateway delivery.
