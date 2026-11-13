# PR-07 Windows Agent-managed profile parity

**Current result:** PASS. Implementation head
`d01e16c3b3c238e135185292eeca2b90152a8a8d` passed the Windows Docker/profile
E2E; documentation checkpoint `721753f9eaf96f9446bf237a840a49677288f5b2`
passed Windows/Ubuntu CI run [43517107187](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43517107187).
Earlier runs below remain as historical evidence.

## Reproduction record

- PR branch head under test: `83e7074be0927dff40829e859a73b6585c720b33`
- Source snapshot SHA-256: `917e6caf2237a680541b13d0a34e2a01f9cefba52ec052b5997a64e5668ae3e5`
- Test: `go test ./cmd/agentctl -run '^TestLiveDockerApplicationProfilesDeliverOTLP$' -count=1 -v`
- Result: `PASS`; test duration 40.40 seconds; Go package duration 40.423 seconds.
- Execution used a genuine elevated Windows PowerShell process. The preflight
  recorded a High Mandatory Level integrity SID, enabled Administrators SID,
  and administrator role membership. No credential ACLs or security checks
  were weakened.
- Runtime: Windows 10.0.26200.0, Go 1.25.5 `windows/amd64`, Docker Desktop
  context `desktop-linux`, Linux Engine 29.5.3 `x86_64`.
- Pinned Collector: `otelcol-contrib` v0.160.0; archive SHA-256
  `d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf`; binary
  SHA-256 `43a7d3eb2b759281b6297a91c830988be39c16e538e8f55aeb4ea31ab7a612cd`.
- Pinned Supervisor v0.160.0 binary SHA-256:
  `df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449`.

## Verified behavior

The Agent-managed Supervisor path delivered real OTLP metrics from Redis,
PostgreSQL, Python/Prometheus, and the mock-cooling fixture. The test output
recorded these metric/resource pairs:

| Profile | Metric | Resource identity |
|---|---|---|
| Redis | `redis.clients.connected` | `node_id=33333333-4444-4555-8666-777777777777`; `resource_id=6975acfce9c1a2067fd14c771e665371b4d2536c5f9ae3e0136a786c229c651f` |
| PostgreSQL | `postgresql.backends` | Same node identity; `resource_id=fc3c5d7dbb23cf8fc09e02f13c31a3be73e2991d63ea295612dc3d2ae27db8d1` |
| Python/Prometheus | `python_app_requests_total` | Same node identity; `resource_id=9f170e01c6224da42629b427ded51fa559d28810a089ff1d1ea87787e57ad398` |
| Mock cooling | `ar_imms_test_cooling_celsius` | Same node identity; `resource_id=594f5d634c35f92d971903cae92fb4524b8c5ce113973c3af6a8cef2b9c416b3` |

The harness passed startup fallback, container churn/recreation, unchanged
Collector configuration and generation during churn, receiver uniqueness,
absence of duplicate Agent-created receiver instances and duplicate samples,
stable node identity across workload recreation, and cleanup. It reported
`new_fixture_containers_after_test=0` and `container_cleanup=PASS`. Pre-existing
containers were not modified.

## Retained local evidence

The untracked local evidence bundle is under
`tmp/pr07-win-profile-83e7074-20261114/evidence-run-2/`:

- `windows-admin-profile-e2e.log` — elevation preflight, binary digests, exact
  command, test output and exit code, and fixture cleanup result.
- `application-profiles/profile-parity-summary.txt` — profile coverage and
  parity result.
- `application-profiles/otlp-sink.log` — sanitized local OTLP sink output.

The tracked evidence here omits the workstation account name and local absolute
paths. The local raw bundle is not part of the release commit.

## Gate boundary

This closes only the Windows Agent-managed Docker/profile parity gate. The
separate default-owner transfer is implemented after this tested head and must
pass final-head Windows/Ubuntu regression and CI before PR-07 can be marked
PASS or merged. Production NestJS Core interoperability remains separate.

## Exact-head retest — 2026-11-14

- Exact PR head: `a6d4f43581cd5f4978884a130a61aeae3760edb2`.
- Exact source snapshot SHA-256: `688b2d04d4905c36c6140419a28563ff4bebf354405d747f5b060b10c0b2e3dc`.
- Test command: `go test ./cmd/agentctl -run '^TestLiveDockerApplicationProfilesDeliverOTLP$' -count=1 -v`.
- Result: **PASS**, 45.84 seconds; exit code 0; zero new fixture containers after cleanup.
- Elevation preflight again confirmed High Mandatory Level and an enabled
  Administrators SID. Docker Desktop `desktop-linux` connected to Linux Engine
  29.5.3 (`x86_64`).
- Collector archive, Collector binary, and Supervisor v0.160.0 SHA-256 values
  match the verified digests above.
- Redis, PostgreSQL, Python/Prometheus, and mock-cooling metrics reached the
  isolated OTLP sink. Node identity and resource identity were recorded for
  each profile. Startup fallback, container churn, receiver uniqueness, and
  cleanup passed.
- Reproducible raw run: `tmp/pr07-win-profile-a6d4f43-20261114/evidence/windows-admin-profile-e2e.log`.
  This is a local ignored artifact and is not part of the release commit.

This retest closes Windows profile parity for the runtime changes on the tested
head. A later PR commit changes only the Linux Docker recovery test retry; the
latest PR head still requires final CI before PR-07 can pass.

## Linux recovery and profile CI — 2026-11-14

- Code head `05b2884e4b442f944390820c8a0413260e2f7057` passed Ubuntu in CI run
  [43507624447](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43507624447).
- The native live Docker test ran a two-network scenario with mapped ports,
  an override port, and an unpublished container. During a real isolated DIND
  outage, inventory remained present and receiver data resumed after restart;
  applied Collector generation remained 2. A paused Redis endpoint was
  reported `endpoint_unreachable` with timestamped Collector evidence.
- The same job passed live OTLP delivery for Redis (`redis.clients.connected`),
  PostgreSQL (`postgresql.backends`), Python/Prometheus
  (`python_app_requests_total`), and mock cooling
  (`ar_imms_test_cooling_celsius`). Receiver uniqueness checks passed after
  churn and recovery. The four-profile managed-runtime E2E, managed Supervisor
  enrollment/remote Gateway E2E, vet, build, and cleanup also passed.
- Ubuntu CI artifact `14077980526` retains sanitized profile/recovery evidence.
  Windows CI passed full tests, native service lifecycle, managed Supervisor
  E2E, vet, and build, but skipped Docker tests. Windows profile parity is
  independently verified by the elevated workstation run above.

At the preceding checkpoint, the remaining PR-07 verification was final-head
CI and review/merge of draft PR #30. The final retest below supersedes that
open-gate note and records the exact current PR head.

## Final PR-head retest — 2026-11-14

- Exact PR head and source snapshot: `d01e16c3b3c238e135185292eeca2b90152a8a8d`.
- Archive SHA-256: `A1D1122D89A0CEAEC041013094D476E1AAFE556C4BC36E813C64AA58131CCBC2`.
- Genuine `RunAs`/UAC execution verified High Mandatory Level, enabled
  Administrators SID, and Administrator role. No DACL or security check was
  weakened.
- Runtime: Windows 10.0.26200.0, Go 1.25.5 `windows/amd64`, Docker Desktop
  `desktop-linux`, Linux Engine 29.5.3 (`x86_64`).
- Collector v0.160.0 archive SHA-256
  `d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf`;
  executable SHA-256
  `43a7d3eb2b759281b6297a91c830988be39c16e538e8f55aeb4ea31ab7a612cd`.
  Supervisor v0.160.0 SHA-256
  `df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449`.
- Command: `go test ./cmd/agentctl -run
  '^TestLiveDockerApplicationProfilesDeliverOTLP$' -count=1 -v`.
  Result: **PASS**, 45.17 seconds, exit code 0; startup fallback, container
  churn, actual OTLP, and cleanup passed; zero fixture containers remained.
- Metrics delivered to the isolated OTLP sink:

  | Profile | Metric | Resource ID |
  |---|---|---|
  | Mock cooling | `ar_imms_test_cooling_celsius` | `84a50c616075c86a5817d0fccac3054ccb55d9851bd9d4ccd3f1d7a0dd8142c7` |
  | Redis | `redis.clients.connected` | `707aad4d838e4bc106281555051e037ac120018303605285c19e9837df6ed409` |
  | PostgreSQL | `postgresql.backends` | `eb500a91398dc99a9ffb73b52f49188a91a3140389ba0a11bbacea34d13122f6` |
  | Python/Prometheus | `python_app_requests_total` | `c26e6cdca07d80b6dd758e6f1f503f5354a2a6ac938f8b856a2efd626b5ec913` |

  All four carried Node ID `33333333-4444-4555-8666-777777777777`. The harness
  reported `owner=agent-managed`, `fallback=PASS`, `container_churn=PASS`, and
  `otlp=PASS`.
- Sanitized execution log SHA-256:
  `009FBF587377D672225F49C8E966858373778A1EC28B78A020EC3A7D0073E97E`.
  Local evidence is retained under
  `tmp/pr07-win-profile-d01e16c-20261114/evidence/` and is excluded from the
  release commit.
- Exact-head Windows/Ubuntu CI run
  [43514647904](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43514647904)
  passed. This closes Windows profile parity for the final PR head.
