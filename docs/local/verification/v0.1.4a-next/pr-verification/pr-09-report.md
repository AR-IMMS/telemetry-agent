# PR-09 Controlled Operations Readiness & Verification

- **Status:** BLOCKED — no approved remote-operations contract or security ADR.
- **Branch:** `feat/pr-09-controlled-operations`
- **Base:** PR-08 merge `d1429196e8b39a24a624a9ad30619476f09a0a9f`
- **Scope completed:** source and contract readiness audit only; no production code, runtime behavior, database schema, Core API, or `docs/shared/` changed.

## Source and contract findings

- The PR-09 plan states that operation names and endpoints are candidates, not an approved catalog. It requires an approved request/response, authorization, target binding, idempotency, audit, timeout, and result-reporting contract before dependent implementation.
- The CLI contract leaves the local-vs-remote mutation authorization model open and says `restart-container` must not ship until the trust model is approved and enforced. Inspection/log output also requires explicit credential redaction and output bounds.
- The cross-repository contract queue has no published remote-operations request/response or authorization/idempotency specification. No contract was found that selects operation allowlist, actor authority, expiry, replay behavior, result envelope, or audit reference.
- `internal/discovery/docker.Provider` has a narrow `engineClient` with only `ContainerList`; it projects inventory summaries and is not a diagnostic or operation executor. No inspect/log/restart executor or operations API is implemented.
- `internal/dependency.Service.Status` reports managed dependency inspection state; it does not provide arbitrary OS service status or restart capability.
- Docker daemon access is high privilege. Adding inspection/logs without an approved data exposure contract could disclose environment-derived secrets; adding restart or service control without authorization policy would exceed the reviewed trust boundary.

## Decision required before implementation

Provide or approve a versioned Operations contract and security ADR that define:

1. Allowed operation catalog and whether the first scope is read-only only.
2. Local CLI versus remote Core request authority, actor identity, authentication, authorization scope, expiry, and opt-in/disable behavior.
3. Immutable target identity and freshness rules, including stale or recreated container behavior.
4. Operation ID/idempotency, durable audit reference, concurrency, deadline, cancellation, and unknown-outcome semantics.
5. Response/error envelope, result reporting, output size/retention, and exact redaction rules for inspect/logs.
6. For mutations, service allowlist, privilege boundary, post-operation verification, and rollback/disable path.

Until those decisions are approved, keep all mutating operations disabled and do not expose new diagnostic or remote operation endpoints. This blocks PR-09 implementation; it does not reopen or block merged PR-08.

## Verification

- Read-only repository/source review was performed on integration head `d1429196e8b39a24a624a9ad30619476f09a0a9f`.
- No code or executable tests were changed or claimed in this readiness audit.
- No contract-dependent scenario is represented as PASS.

## Rollback and next step

No runtime change exists to roll back. After the contract/ADR is approved, re-baseline the source, define the allowed first slice, then implement its contract tests and platform verification. Keep this checkpoint BLOCKED until those prerequisites exist.
