# PR-06 verification report — application monitoring profiles

**Status:** VERIFYING — Windows live delivery passes; native Linux exact-binary and Docker/OTLP E2E are pending CI on the proposed PR head.
**Release target:** `v0.1.4a-next`
**Branch:** `feat/pr-06-monitoring-profiles`
**Base:** `chore/v0.1.4a-next` at PR-05 merge `11fdd1e720c542898ba9c0c673063b1d698d7451`
**PR / commit:** [Draft PR #29](https://github.com/AR-IMMS/telemetry-agent/pull/29), head `d4136c870186be2ca405f490d0678a8955ad63f2`.

## Scope delivered

- Added bounded, versioned Agent profile files at `<config-root>/monitoring-profiles/`; unknown fields and unsupported receiver/capability pairs are rejected. The profile schema cannot contain raw Collector YAML, OTTL expressions, plugins, or code.
- Kept Redis as the built-in profile and added typed PostgreSQL and Prometheus projection through the pinned OTel receivers. Existing Docker Observer and Receiver Creator remain the only discovery and dynamic receiver lifecycle components.
- Added PostgreSQL environment-variable references and optional trusted `tls_ca_file`. Secret values are not stored in profile files, Agent state, or diagnostics. A PostgreSQL receiver rule is omitted until both credential values are available; inventory inspect reports `requires_configuration`.
- Connected the configured registry through bootstrap, reconciliation ownership, template materialization, and inventory diagnostics. Added Python and PostgreSQL examples plus a mock cooling profile in the live E2E fixture.
- Added pinned Collector manifest entries and component-module checks for PostgreSQL and Prometheus.

## Windows verification

Environment: Windows/amd64, Docker Desktop Engine 29.5.3, pinned `otelcol-contrib 0.160.0` Windows archive SHA-256 `d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf`. The real Docker test ran Redis 7, a PostgreSQL 17 fixture with TLS and a `pg_monitor`-granted metrics user, Python `/metrics`, and a separately configured `mock-cooling` profile. The test used an isolated OTLP receiver/sink and a generated trusted test CA.

| Verification | Result | Evidence |
|---|---|---|
| Full Windows Go suite | PASS | `go test ./... -count=1`, pinned binary provided; all packages passed. |
| Pinned profile candidate validation | PASS | `TestPinnedCollectorValidatesLocalPostgreSQLAndPrometheusProfiles` validated generated PostgreSQL and Prometheus configuration against v0.160.0. |
| Redis actual OTLP delivery | PASS | `redis.clients.connected` arrived with expected Node and container resource identity. |
| PostgreSQL actual OTLP delivery | PASS | `postgresql.backends` arrived through TLS-verified PostgreSQL with restricted `pg_monitor` credentials. |
| Python `/metrics` delivery | PASS | `python_app_requests_total` arrived through the generic Prometheus profile. |
| Config-only mock cooling delivery | PASS | New `mock-cooling` profile produced `ar_imms_test_cooling_celsius` without changes to generic discovery logic. |
| Receiver ownership and churn | PASS | One active Receiver Creator instance per target; Agent desired Integration set remained empty. Recreating the mock workload changed `resource_id` and delivered data without changing Collector config bytes or applied generation. |
| Duplicate telemetry | PASS | Test checked duplicate metric/timestamp samples for each profile and found none. |
| Test fixture cleanup | PASS | Disposable containers and the generated image were removed by test cleanup. |

Reproduction command:

```powershell
$env:AR_IMMS_PR06_APPLICATION_PROFILE_E2E = '1'
$env:AR_IMMS_PR05_COLLECTOR_BINARY = '<checksum-verified otelcol-contrib 0.160.0 path>'
$env:AR_IMMS_DOCKER_DISCOVERY_ENDPOINT = 'npipe:////./pipe/docker_engine'
go test ./cmd/agentctl -run '^TestLiveDockerApplicationProfilesDeliverOTLP$' -count=1 -v
```

The retained command output is [pr-06-windows-live-output.txt](./pr-06-windows-live-output.txt). It records image IDs and one `PR06_OTLP_DELIVERED` marker for each metric/resource pair.

## Other local gates

| Check | Result |
|---|---|
| `go test ./... -count=1` | PASS on Windows. |
| `go vet ./...` | PASS on Windows. |
| `go build ./...` | PASS on Windows. |
| `GOOS=linux GOARCH=amd64 go build ./...` | PASS from Windows; this is a cross-build, not Linux E2E evidence. |
| Architecture/import/450-line guard | PASS; 32 source directories and 293 production Go files checked. |
| Native Linux full tests and live Docker/Collector/OTLP | PENDING PR CI; Windows build is not Linux E2E evidence. |

Exact-head CI run [43272599179](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43272599179) passed Windows, the Linux full suite, and the PR-05 live Redis gate, but failed the Linux application-profile E2E before Collector activation: the empty isolated Docker daemon did not have `postgres:17-alpine` when the test inspected fixture image IDs. Windows had cached the image, which masked this fixture dependency. The E2E now explicitly pulls Redis, PostgreSQL, and Python fixture images before recording their image IDs; the updated Windows live E2E passed in 55.85 seconds. The PR remains draft pending a fresh exact-head CI run of the corrected fixture, including native Linux application-profile OTLP delivery.

The full Go suite was rerun with normal host filesystem and loopback access after a sandbox-restricted run failed to create SQLite state and bind test sockets. The host run passed all packages. The opt-in Windows Docker E2E was rerun against the pinned binary and passed in 33.84 seconds, delivering all four expected metric families and checking receiver uniqueness, duplicate samples, and churn convergence.

## Open decisions and limitations

- Core-versus-local profile/config precedence and remote secret delivery remain OPEN; this checkpoint only reads local files and the Agent service environment.
- Operators must provision the PostgreSQL environment variables to the Agent service. The Agent does not add a credential CLI or define remote secret lifecycle.
- Native Linux E2E and Linux pinned binary component/config validation must pass before merge. If the isolated Linux fixture or artifact gate fails, keep the PR unmerged and update this report with exact evidence.

## Rollback and next checkpoint readiness

Rollback is a PR revert plus removal of the local profile files; Collector candidate validation and existing LKG recovery protect the active configuration. No SQLite migration, shared-submodule edit, or Core change was made.

PR-06 is not PASS until native Linux full tests, pinned PostgreSQL/Prometheus component and config validation, and real Linux Docker-to-OTLP evidence pass on the exact proposed PR head. Keep the PR draft until those mandatory gates pass. PR-07 may begin only after the verified PR-06 merge.
