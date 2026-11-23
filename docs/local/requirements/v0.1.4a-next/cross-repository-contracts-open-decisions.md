# Cross-repository contract publication queue

**Not yet published to `docs/shared/`.** Core and Agent owners jointly review
and approve before publishing any cross-repository contract. The PR-07
proposal narrows the initial Core-owned policy to a typed, versioned OTLP
Gateway endpoint; this is a proposed scope, not an approved schema.

Before PR-07b depends on these contracts, inspect the authoritative enrollment
OpenAPI and Core API Baseline, then resolve:

1. Installation/binding ↔ OpAMP `instance_uid` mapping; Supervisor credential
   provisioning, storage, rotation, revocation, and session invalidation.
2. Target, schema version, desired revision, hash canonicalization, stale/equal/
   conflict behavior, local/remote field ownership, deterministic merge order,
   and deletion semantics for the Gateway endpoint policy.
3. Status/error mapping from native OpAMP and actual Collector runtime evidence;
   desired revision must remain distinct from applied revision, and readiness
   must not claim OTLP delivery.
4. Internal Core HTTP operations for authentication, identity binding, desired
   policy, config status, and health, including response/error envelope, request
   IDs, auth, idempotency, retry, cache, and reconnect semantics.
5. Production NestJS Core interoperability and shared publication approval.

Also retain the separately gated typed remote-operations request/response and
authorization/idempotency specification. Do not invent an OpenAPI schema for
native OpAMP Protobuf or treat Go Mock Core conformance as production Core E2E.
This local queue records decisions required; it is not a published contract.
