# Agent Operations Contracts

**Version:** `ar-imms.operation.request.v1` / `ar-imms.operation.result.v1`  
**Status:** Agent-local implementation contract for PR-09 Phase 1; production Core interoperability remains a separately approved integration.

Core is the business authorization authority. The Agent validates message version, authenticated broker scope, installation and binding identity, expiry, supported operation type, target identity, and local execution safety. `requested_by` and `authorization_ref` are audit correlations only; the Agent does not evaluate human roles.

The initial read-only catalog is `agent.diagnostics`, `container.inspect`, and `dependency.inspect`. Mutations are excluded from this contract. `container.logs` is excluded because its sensitive-data exposure policy is unresolved.

The schemas reject unknown fields and bound envelopes, target identifiers, parameters, and results. Operation IDs are immutable. Fingerprints use JSON encoding of the typed envelope with parameter object keys canonicalized by Go's JSON encoder; semantically different field values remain different requests. A repeated ID and fingerprint reuses its stored outcome; a repeated ID with a different fingerprint is rejected. Actor and authorization references are retained in the local audit trail for correlation and are not interpreted as business permissions.

Phase 1 uses a separate installation-scoped process lock for the Operations consumer. It recovers durable `RUNNING` read-only executions only after obtaining that lock, closing each as `FAILED` with `EXECUTION_INTERRUPTED`, committing audit/outbox evidence, and not replaying handler work. This avoids another Agent process finalizing a live execution and avoids claiming exactly-once execution or silently repeating work before Core approves retry semantics. Handler execution is bounded to 30 seconds and JetStream ACK progress is extended during work. `container.restart` is not part of this catalog and has no enabled handler.

See [request schema](operation-request-v1.schema.json), [result schema](operation-result-v1.schema.json), and the [JetStream contract](transport.md). These local specifications do not modify or supersede `docs/shared/` contracts.
