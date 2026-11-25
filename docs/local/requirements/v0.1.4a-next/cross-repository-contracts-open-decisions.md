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

## Secure OTLP ingestion contract proposal — 2026-11-25

This additive proposal records the locally implemented and UAT-verified
credential boundary. It is **not approved or published** to `docs/shared/`.

- Keep the existing OpAMP credential and add a distinct random bearer
  credential scoped to `otlp.write`. Fleet owns issuance, expiry/status,
  verification, and revocation. Agent stores the secret in its protected
  credential store and provisions a restricted Collector token file.
- Put the OTLP secret in a secret-bearing enrollment response only; store a
  verifier in Fleet and keep replay/recovery behavior explicit. Do not store
  raw credentials in SQLite, logs, diagnostics, or rendered Collector YAML.
- Collector sends OTLP/HTTP JSON over TLS to the only reachable OTLP ingress.
  The ingress performs a per-request authenticated Fleet lookup, requires the
  `otlp.write` scope, rejects inactive/expired credentials, replaces any
  client-supplied `agent_id` with the authenticated Fleet principal, and
  forwards only to the private Gateway listener.
- Use an authenticated TLS service identity for ingress-to-Fleet lookups. The
  lookup returns identity, scope, status, and expiry, never the bearer. Do not
  cache successful authorization responses; Fleet unavailability fails
  closed. Core remains the human/Node authorization boundary and requests
  Fleet lifecycle operations through its trusted service channel.
- Revoke both the OpAMP and OTLP credentials for an Agent binding. With no
  positive cache, the residual Gateway enforcement bound is Fleet commit
  visibility plus any request already authorized and in flight, limited by
  the ingress 10-second request deadline.
- Local UAT passed the full Core/Fleet/Agent/official-Supervisor/Collector/TLS
  ingress/Gateway path, identity replacement, Core synchronization/revocation,
  and same-token HTTP 401 denial. See
  [`secure-otlp-supervisor-uat.md`](../../verification/secure-otlp-supervisor-uat.md).

Owners must still approve the additive enrollment schema, private lookup path
and response fields, service-auth identity, token expiry and retry semantics,
Fleet outage behavior, and revocation propagation statement. Update the
authoritative OpenAPI and shared C01/C04 docs only after Core and Fleet owners
review the proposal. Exact-head hosted CI and live Windows runtime verification
also remain pending.
