# ADR 0003: Read-only Agent Operations foundation

**Status:** Accepted for PR-09 Phase 1 by the user on 2026-11-14  
**Scope:** Read-only operations only; no production Core changes and no `docs/shared/` changes.

## Context

The Agent needs a bounded way to receive Core-authorized diagnostic requests, execute locally supported read-only capabilities, retain execution evidence, and report results after temporary broker outages. Existing enrollment, OpAMP, OTLP, SQLite, Health, Docker, and managed-dependency behavior must remain independently owned.

## Decision

- Use versioned `ar-imms.operation.request.v1` and `ar-imms.operation.result.v1` local contracts and the per-installation JetStream subjects documented in `contracts/operations/`.
- Core remains the sole business authorization authority. The Agent checks transport authentication, operation/version allowlists, installation/binding identity, expiry, exact target identity, output bounds, and execution idempotency. Actor and authorization identifiers are correlations only.
- Operations owns typed handler registration, common request validation/idempotency, execution state, local audit, result publication state, and recovery coordination. Handlers consume existing Health, Docker inspection, and managed-dependency capabilities through narrow ports. NATS and SQLite adapters remain outside handlers.
- Persist claims in the existing operational SQLite database. Commit terminal execution, final audit, and one result outbox event atomically. ACK commands only after durable terminal completion; publish result events with stable IDs and mark them published only after PubAck.
- Phase 1 includes only `agent.diagnostics`, `container.inspect`, and `dependency.inspect`. Container inspection requires a full Docker ID and returns a structured allowlist. Dependency inspection is limited to the Agent catalog. No arbitrary logs, shell, service manager inspection, or mutation is exposed.
- NATS is optional and isolated from telemetry availability. It requires TLS and per-binding credentials; its outage cannot stop the current Collector, reconciliation, or telemetry loops.
- `container.restart` remains disabled and is not part of the catalog. Phase 2 requires a separately approved operation-specific safety contract before code or enablement.

## Consequences

The local transport and schemas are not a production Core contract. Mock Core can validate protocol behavior without modifying NestJS Core. Broker credential provisioning and production interoperability remain separate integration work. JetStream is at-least-once; SQLite execution identity and Core result deduplication prevent duplicate local execution and duplicate result acceptance, but do not claim exactly-once delivery. SQLite audit is append-only by application behavior, not cryptographically tamper-proof.

## Verification gates

Use real JetStream and native Windows/Linux execution. Verify TLS/auth, installation isolation, durable delivery/redelivery, DB commit before ACK, result PubAck/outbox retry, broker recovery, duplicate identity, expired/wrong-binding rejection, secret-free Docker projection, and telemetry continuity. Record unrun gates explicitly in the PR-09 report.
