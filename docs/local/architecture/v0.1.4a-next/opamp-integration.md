# OpAMP integration — NestJS Core / Go Agent

**Decision:** NestJS adapter **in Core**, Go OpAMP client **in Agent**; initial authenticated plain-HTTP polling. Full OpAMP implementation is out of scope. **Status:** protocol integration and config-authority semantics remain subject to conformance and ADR review.

## Logical ownership

Core Agent Registry verifies installation binding and revocation; configuration publication owns targeted versioned AR-IMMS policy snapshots; transport adapter handles Protobuf messages and negotiated protocol capabilities. Agent Connectivity owns OpAMP client session/retry, while Configuration validates/persists desired policy and Runtime applies Collector generation. Health projects native remote-config apply statuses and separate AR-IMMS runtime/telemetry evidence.

## Happy path

1. Agent enrolls through existing Core enrollment API, securely stores installation credential.
2. Agent polls OpAMP over authenticated TLS with instance UID mapped to (not equated with) stable Installation ID.
3. Core returns `remote_config` with versioned AR-IMMS policy document in `config_map`, hash and declared content type.
4. Agent checks signature/authentication/trust, size, schema, capability and revision/target, persists accepted policy atomically.
5. Existing reconciliation compiles supported Collector configuration, validates, activates generation or restores LKG.
6. Agent reports `remote_config_status`/effective config and health appropriately; Core records actual status, not a fabricated delivery guarantee.

## Negative paths

Stale/replayed hash, unauthorized installation, duplicate polls, omitted optional fields, network disconnect, malformed Protobuf, huge config, unsupported capability, secrets accidentally embedded, Core restarted, Agent restarted, hash mismatch, apply failure, partial status response, changing instance UID, Gateway down. None may corrupt existing LKG or silently overwrite unrelated local state.

## Open decisions that must precede broad rollout

**Core/local authority** (namespace, overrides, deletions), full-vs-partial snapshot and revision hash canonicalization, credential source/scope, revocation-offline semantics, status retention, detailed protocol version/conformance evidence. Keep a single isolated Core-owned test namespace until decided. Do not confuse OpAMP Agent restart with Docker container restart; operations are separate API.

## References

- https://opentelemetry.io/docs/specs/opamp/
- https://github.com/open-telemetry/opamp-spec
- https://github.com/open-telemetry/opamp-go
