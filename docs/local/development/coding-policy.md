# Telemetry Agent — repository-specific coding and commenting policy

**Status:** proposed supplement to `docs/shared/engineering/AI_CODING_POLICY.md`; if provisions conflict, seek explicit architecture owner review. Do **not** silently create two competing authoritative policies.

## Code architecture

- One owning bounded context per domain entity; consumer-owned small interfaces, DI at composition root; domain unaware of SQLite/HTTP/OS/OTel SDK.
- No `cmd/agentctl` business orchestration or direct SQLite table access; no God `common`/`utils` package.
- Keep existing contracts stable through refactor; explicit migration + regression for changes. Do not mix package moves with runtime behavior, schema changes or component swaps.
- Keep source files generally within 300–400 lines, consistent with
  [`docs/shared/engineering/AI_CODING_POLICY.md`](../../shared/engineering/AI_CODING_POLICY.md);
  justify larger files. A 450-line automated ceiling, if adopted, is a hard
  stop and does not waive the shared policy's justification for files above
  the normal range. Avoid meaningless splits solely to satisfy a metric.
- Prefer standard Go libraries, existing dependencies and OpenTelemetry components; no homegrown receiver/scraper/OpAMP protocol implementation without necessity and a design approval.

## Comments and documentation

- Every context: `doc.go` with purpose, owned models, interfaces, forbidden dependencies, lifecycle and security boundary.
- GoDoc on every exported type/function/method. State guarantees, errors, side effects and required caller behavior where non-obvious.
- Explain domain invariants, concurrency/locks, transaction atomicity, recovery, resource identity, credentials, OS differences, and OTel capability assumptions.
- Do not comment obvious syntax. Do not make unsupported promises like “zero loss”, “atomic” or “read-only Docker access” without verified contract.
- Add decision record when modifying authority, receiver ownership, security/privilege, or storage format.

## Operational safety

- Thread `context.Context` through I/O; bounded goroutines/queues/timeouts; no silent retries for non-idempotent mutation.
- Credentials never in logs/events/CLI output; redaction, TLS, least privilege, explicit Docker socket hazard analysis.
- Domain writes transactional with CAS where required. Publish typed events after commit; retain fallback poll/reconcile.
- Unknown outcomes must be reported accurately (e.g. timeout vs success), and runtime readiness is not telemetry delivery.

## Mandatory checks per PR

`gofmt`, `go test ./...`, `go vet ./...`, architecture import rules, ≤450-LOC gate, applicable race tests, Windows and Linux builds, SQLite old fixture migration/reopen/CAS. For behavior-bearing changes: real Collector binary validation, live OTLP evidence where feasible; explicitly label NOT RUN/BLOCKED if unavailable. No pretend E2E. Every PR includes plan, test/verification report, affected contracts, rollback and evidence artifacts.

## PR review checklist

Why this abstraction? Can OTel do it? What owner/source-of-truth? What changes on crash/restart? What's the OS difference? Can it duplicate a receiver? Is security scoped? Are comments accurate? Are current CLI and SQLite data compatible?
