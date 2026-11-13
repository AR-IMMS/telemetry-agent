# PR-07 Windows Agent-managed profile parity

**Result:** PASS for Windows Docker/profile parity on the tested PR head. PR-07
remains IN_PROGRESS pending verification of the subsequent default-owner code
change on its final commit and CI.

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
