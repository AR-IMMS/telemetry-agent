# Handoff: S3 Integration Reconciliation and Redis — Slice 3

## Current state

- Branch: `feat/s3-slice1-unified-lifecycle`.
- Slice 3 implementation, regression tests, and CI workflow changes are committed locally; no push or PR has been made yet.
- The Target Architecture v0.1.4 remains unchanged and is the architecture source of truth.
- Slice 3 status: **accepted with limitations**. Windows/Linux artifact and test verification passed. GitHub Actions and installed Windows Service execution remain unverified in this environment.

## Read these artifacts

- `docs/local/architecture/target-architecture-v0.1.4.md` — approved architecture source of truth.
- `docs/local/architecture/integration-core-s1-design.md` — S1 domain, registry, resolver and persistence design.
- `docs/local/architecture/s2-collector-capability-compiler-design.md` — S2 catalog, candidate compilation and validation design.
- `docs/local/architecture/s3-reconciliation-capability-verification-design.md` — S3 reconciliation and capability verification design.
- `docs/local/architecture/s3-reconciliation-redis-design.md` — S3 boundaries, assembly ownership, Redis example and unresolved risks.
- `docs/local/roadmap/telemetry-agent-v0.1.4-integration-core-slices.md` — Integration Core milestone history/context.

## Delivered behavior

- The production registry only includes the Redis definition when the pinned platform catalog contains `otel.receiver.redis.metrics`.
- Redis v1 validates a host-and-port endpoint and rejects unsupported settings, including plaintext password fields. It contributes an immutable endpoint-only intent and does not connect to Redis or own runtime behavior.
- Capability IDs accept dotted OTel names while retaining validation. The static manifest enables Redis only for verified Linux/Windows amd64 Collector 0.160.0 artifacts; unsupported architectures fail closed.
- CI downloads the pinned Windows/Linux archives, checks SHA-256 and version, inventories components, then runs native-validation and repository tests. Windows archive extraction passes a relative archive name to avoid GNU tar treating a drive-letter path as a remote host.
- Reconciliation and M2 remain integration-agnostic. A regression test exercises Redis plus a PostgreSQL-like fixture and managed dependency through the existing complete-candidate lifecycle path.

## Slice 3 commits

- `c4f8ddb feat(integration): add capability-gated Redis definition`
- `89fb68c feat(collectorruntime): enable verified Redis capability`
- `ea6fb9e test(reconciliation): cover Redis plug-unplug convergence`
- `ad8b7e7 ci(collector): verify pinned artifacts on Windows and Linux`

The feature branch also contains the previously completed candidate lifecycle and reconciliation commits; inspect `git log --oneline origin/dev..HEAD` for the full branch stack.

## Verification recorded

- Windows: `go test ./... -count=1`, focused S1/S2/S3 tests, `go vet ./...`, and `go build -buildvcs=false ./...` passed.
- Windows Redis E2E passed with a real Redis 7.4.2 container, production `agentctl run`, isolated SQLite, pinned Collector 0.160.0 and an OTLP Gateway. Desired revisions 1–4 reached Applied (generations 2–5); Redis and host metrics reached the Gateway; host metrics continued while Redis was unavailable and after disable/delete.
- Linux amd64 artifact SHA-256/version, Redis receiver inventory (`receiver/redisreceiver v0.160.0`), native candidate validation, and focused Go tests passed in Docker using the pinned artifact.
- `gofmt`, scoped `git diff --check`, local relative-path `tar.exe` extraction probe, and source architecture reviews passed.
- Evidence files are under `.local-verification/s3-slice3/`; they are local-only and were not included in implementation commits.

## Remaining limitations and next steps

1. Run the GitHub Actions workflow on the pushed branch/PR. It was not run locally; this workflow triggers for pushes to `main`/`dev` or PRs targeting those branches.
2. Verify installed Windows Service startup, polling, activation and restart recovery in an elevated isolated environment. This session was not an administrator and could not open the Service Control Manager; no host service was installed or changed.
3. Keep custom Collector distributions, other architectures, Discovery and remote ownership policies out of this milestone. Redis capability remains fail-closed outside the two verified amd64 artifacts.

Configuration Applied is not evidence of Redis health or Gateway delivery; E2E Gateway metric evidence is recorded separately from Collector readiness and desired-state status.
