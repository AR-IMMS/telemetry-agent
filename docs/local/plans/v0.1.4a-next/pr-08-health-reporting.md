# PR-08 — Health & Reporting Implementation Plan

> **For agentic workers:** Use `superpowers:executing-plans` to implement this plan task by task. Each task is a separately reviewable checkpoint; do not begin PR-08 before PR-07 passes and merges.

**Goal:** Provide truthful, evidence-based Agent health, local diagnostics, and supported status reporting without taking ownership of recovery or telemetry execution.

**Architecture:** Health owns evidence collection, freshness/completeness evaluation, assessment, and diagnostics. Runtime, Identity, Inventory, Monitoring, and Configuration remain the owners of the facts and mutations in their domains. Core reporting must consume only an approved contract; do not invent endpoints or status semantics.

**Tech Stack:** Go 1.25.5, existing SQLite/event and Agent runtime evidence, Windows and Linux native test environments, pinned OTel Collector and OpAMP Supervisor v0.160.0.

**Spec:** [Release scope](../../requirements/v0.1.4a-next/release-scope.md), [acceptance criteria](../../requirements/v0.1.4a-next/acceptance-criteria.md), [context boundaries](../../architecture/v0.1.4a-next/context-boundaries.md), [OpAMP integration](../../architecture/v0.1.4a-next/opamp-integration.md), and the mission's Health context and PR-08 requirements. Reconcile these documents against current source and approved Core contracts before implementation.

## Global Constraints

- Keep the seven bounded contexts and the dependency direction in the accepted release mission.
- Health evaluates evidence; it does not mutate configuration, restart components, or own their persistence.
- Separate Agent, Core, Supervisor, Collector, configuration, inventory, monitoring, dependency, and delivery status.
- Missing, stale, incomplete, and unsupported evidence must remain distinguishable from healthy evidence.
- Collector readiness and OpAMP status do not prove successful collection or OTLP delivery.
- Reuse existing OTel/runtime evidence; do not add a parallel telemetry pipeline.
- Bound diagnostic storage, payloads, retries, and retention; redact credentials and protected configuration.
- Preserve CLI compatibility, SQLite migration/order/CAS guarantees, Windows/Linux behavior, and managed-runtime ownership.
- Do not modify `docs/shared/` or Core production code without authorization.

## Review Focus

- A running process with an unavailable Collector must not be reported as fully healthy; test each component dimension independently.
- Stale or missing timestamps must yield an explicit unknown/stale result, not a fabricated success; test freshness boundaries.
- Collector readiness without an observed sink delivery must not set delivery healthy; retain a real OTLP evidence test.
- Health must preserve local evidence and must not own recovery. Test Core, OpAMP, and Gateway outage/recovery only through existing owner evidence; do not add probes or invent endpoints.
- Core connectivity evidence must distinguish network failure, authentication failure, and unavailable/stale evidence. OpAMP session evidence is independent of Core availability.
- Gateway connectivity may use supported Collector exporter diagnostics and telemetry evidence. Collector readiness alone proves neither Gateway connectivity nor successful OTLP receipt.
- Prefer existing evidence sources: authenticated Core API request outcomes for Core connectivity, Supervisor/OpAMP session evidence for OpAMP connectivity, and supported Collector exporter diagnostics for Gateway connectivity. Do not introduce a probe or status channel to manufacture missing evidence; report `UNKNOWN` when a source is absent or stale.
- External Core reporting is deferred until its contract is approved and does not block local health snapshots, evidence evaluation, or reporting ports. Contract-dependent outage/recovery cases remain `BLOCKED` or `NOT_TESTED` for release integration, with the exact missing contract/evidence source recorded.
- Diagnostic snapshots must omit tokens, credentials, private keys, and protected configuration values; test serialized outputs and logs.

---

## Task 1: Baseline health evidence contracts

**Files:**

- Read: `internal/agenthealth/`, owning Runtime/Identity/Inventory/Monitoring/Configuration packages, `docs/local/architecture/v0.1.4a-next/`, and existing PR-07 evidence.
- Modify: `docs/local/architecture/v0.1.4a-next/context-boundaries.md` only where the approved seven-context terminology or Health contract is missing.
- Create or update: `docs/local/verification/v0.1.4a-next/pr-verification/pr-08-report.md`.

**Interfaces:**

- Consumes: existing typed evidence from the owning contexts and any reviewed Core reporting contract.
- Produces: a source-checked evidence matrix defining owner, type, timestamp/freshness, completeness, error/unknown semantics, and sensitive fields for each health dimension.

- [ ] Inventory current source and tests; identify exactly which evidence is already emitted and which is absent.
- [ ] Write contract tests for the existing or approved evidence DTOs and their freshness/completeness semantics before implementation.
- [ ] Record unresolved Core reporting fields, retry, idempotency, and persistence decisions as open; do not infer a wire format.
- [ ] Run focused source and contract tests; record exact HEAD and whether each case is verified, unsupported, or blocked.

## Task 2: Health assessment and local diagnostics

**Files:**

- Modify: the source-identified Health owner, expected to include `internal/agenthealth/` unless the PR-09.5 ownership audit identifies a justified target.
- Test: the corresponding Health package tests and integration test package identified from current repository structure.

**Interfaces:**

- Consumes: Task 1 evidence contracts.
- Produces: local snapshots and an assessment with separate statuses for Agent runtime, Core connectivity, OpAMP session connectivity, Supervisor, Collector, Gateway connectivity, configuration, inventory, monitoring, dependencies, and telemetry delivery. Missing or stale evidence is `UNKNOWN`.

- [ ] Add tests for independently healthy/unhealthy components, stale/missing evidence, and incomplete provider observations.
- [ ] Implement assessment without direct process, configuration, database, Docker, or service-manager mutation.
- [ ] Add local diagnostics with explicit evidence timestamps and sanitized values.
- [ ] Verify credential/configuration redaction and bounded output with adversarial fixtures.
- [ ] Run focused tests, architecture guard, `go vet ./...`, and `go build ./...`.

## Task 3: Core health reporting, only when contract is approved

**Files:**

- Modify: the existing Agent-to-Core adapter and Health-owned application boundary identified by the contract review.
- Test: schema/contract tests and Mock Core integration tests; do not modify Core production code or `docs/shared/`.

**Interfaces:**

- Consumes: the approved versioned reporting contract, or remains unimplemented if no approved contract exists.
- Produces: bounded, retryable, idempotent reports with explicit unknown/error status and no secret material.

- [ ] Confirm owner, version, request/response, authentication, status vocabulary, retry limits, idempotency, and retention before dependent code.
- [ ] Add positive, rejection, outage, duplicate-report, stale-report, and restart/replay contract tests.
- [ ] Implement only the approved adapter behavior; keep local health evaluation usable offline.
- [ ] Label Mock Core verification separately from production NestJS interoperability.

## Task 4: Failure and recovery verification

**Files:**

- Test: existing Agent lifecycle, Supervisor, Collector, Docker, and OTLP E2E harnesses; add only focused fixtures needed for evidence.
- Update: `docs/local/verification/v0.1.4a-next/pr-verification/pr-08-report.md` and `docs/local/plans/v0.1.4a-next/release-progress.md`.

- [ ] Verify Agent, Supervisor, and Collector failure/recovery using existing lifecycle evidence; verify Core, OpAMP, and Gateway outage/recovery only where existing owner evidence is exposed.
- [ ] Record Core/OpAMP/Gateway cases as `BLOCKED` or `NOT_TESTED` when required owner evidence or an approved contract is absent; state the exact missing source/contract and follow-up.
- [ ] Do not introduce Core health-report APIs, custom Gateway probes, or a reporting schema. Defer external Core reporting until its contract is approved.
- [ ] Verify profile/receiver failure while the Collector process remains healthy.
- [ ] Verify configuration rejection/LKG recovery and status convergence without Health initiating recovery.
- [ ] Run native Windows and Linux tests; distinguish local tests, mocks, hosted CI, and real infrastructure E2E.
- [ ] Record exact commit, commands, artifact digests, sanitized evidence, limits, and PR-09 readiness.

## Exit Criteria

- Each health dimension has a named evidence owner and explicit freshness/completeness semantics.
- Local diagnostics are bounded and secret-free.
- No health result claims collection or delivery based only on process/configuration/readiness evidence.
- External Core reporting remains deferred until its contract is approved; local snapshots and reporting ports work independently and do not invent a wire contract.
- Failure and recovery tests execute natively on Windows and Linux for supported scenarios.
- PR-08 report and release tracker match the exact implementation and verification evidence.
