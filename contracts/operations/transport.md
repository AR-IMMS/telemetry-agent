# JetStream Transport Contract

**Version:** `arimms.ops.v1`  
**Agent client:** `github.com/nats-io/nats.go v1.50.0`  
**Provisioning:** Core or deployment infrastructure provisions streams, consumers, and per-installation/binding credentials. Agent does not administer broker resources.

## Broker binding provisioning v1

`ar-imms.operations.broker-binding.v1` is the minimal secret-free binding record
used by Core and the test Mock Core. Its JSON Schema is
[broker-binding-v1.schema.json](broker-binding-v1.schema.json). It identifies
the installation and binding, carries a monotonically increasing binding
revision, and reports `active` or `revoked`. An active record contains the TLS
endpoint, binding-scoped username, protected `credential_ref`, CA path, request
stream, and durable. It never contains a password or token; unknown fields are
rejected. The mock enforces increasing revisions and clears broker identity
fields on revocation.

Core/deployment provisions a distinct NATS username/password and exact ACLs for
each active binding, then places the password under the provided opaque
reference in the Agent's protected credential store. The opaque reference stays
stable for that binding; rotation advances the binding revision and atomically
replaces the protected value at that reference. Revocation advances the
revision, removes the protected value, and removes the broker user/permissions.
The Agent polls only its local protected binding state; if the credential
disappears or the binding is revoked, it closes the existing Operations
connection immediately and retries without that binding. A replacement
credential causes the old session to close before the new one connects. This
local session invalidation is separate from broker-side auth reload: NATS
configuration reload alone is not assumed to terminate an already-authenticated
connection. The URL uses the Go NATS client's `nats://` form while the Agent
always enables TLS and validates the configured CA. Operations revocation does not stop monitoring, collection,
telemetry export, or Collector lifecycle.

The contract defines the local Core/Agent provisioning handoff and test Mock
Core behavior; it does not add a production Core endpoint or claim production
NestJS interoperability. That API remains separately tracked for its owning
checkpoint.

## Subjects and durability

| Direction | Subject | Stream | Handling |
|---|---|---|---|
| Core → Agent | `arimms.ops.v1.request.<installation_id>` | `ARIMMS_OP_REQUESTS` | Installation-specific durable pull consumer, explicit ACK |
| Agent → Core | `arimms.ops.v1.result.<installation_id>` | `ARIMMS_OP_RESULTS` | Persisted result outbox, JetStream PubAck before local publish completion |

Streams use persistent storage and bounded retention/maximum age coordinated with command expiry and the supported offline window. The Agent checks expiry on receipt. Each installation/binding lifecycle receives an exact subject filter and a unique durable consumer. Agent uses the already provisioned consumer, a single in-flight pull, manual ACK, and bounded redelivery configured by the operator. A broker redelivery never bypasses the SQLite execution identity.

The Agent publishes an outbox result using its stable `event_id` as JetStream message ID. If PubAck is lost or the process exits before marking publication, the Agent retries the same event ID and payload. Core/Mock Core deduplicates by event ID. This provides at-least-once transport with idempotent result acceptance; it does not claim exactly-once execution or publication.

## Authentication and authorization

Connections require TLS 1.2 or later, CA validation, and a dedicated user/password pair provisioned for the installation/binding. The enrollment bearer is not reused as a broker credential. Credential material is supplied through protected service configuration and is never written to logs or operation records. The Agent starts Operations only when all required broker settings are present; absent or unavailable Operations must not stop telemetry and Collector lifecycle.

The Agent identity in the envelope is checked against the installation ID read from the existing SQLite identity row and the configured current binding. Subject identity and envelope identity are independently checked. Replacement bindings receive distinct credentials/consumers; stale queued commands fail the binding check.

Broker policy permits the Agent to consume only its own command subject and publish only its own result subject, plus the minimal JetStream pull, acknowledgment, and reply-inbox API subjects required by the pinned client/server. It denies command publication, other installations' request/result subjects, arbitrary stream/consumer administration, stored-message access, and unrelated AI/event subjects. Verify these ACLs against the actual pinned server in the native testbed; do not grant broad `$JS.API.>` permissions.

## Acknowledgment and recovery

- Unsupported/malformed payloads are terminally acknowledged with a bounded reason and never logged with the payload.
- A valid request is ACKed only after its terminal result, audit event, and outbox row commit atomically in SQLite.
- If SQLite claim or completion fails, the message is negatively acknowledged with delay; it is not ACKed as successful.
- An in-progress duplicate is negatively acknowledged; a terminal duplicate reuses the stored result without invoking the handler again.
- The Agent holds one installation-scoped Operations process lock before startup recovery or command consumption. After acquiring it, leftover `RUNNING` read-only records are atomically finalized as `FAILED` / `EXECUTION_INTERRUPTED`; interrupted `container.restart` records are finalized as `UNKNOWN` / `OPERATION_OUTCOME_UNKNOWN`. Both include audit and outbox evidence. An uncertain restart is never automatically retried, including after Agent restart; redelivery of the same operation ID returns the persisted result.
- Handler execution is bounded to 30 seconds or the request expiry, whichever comes first. ACK progress is extended during handler execution so broker redelivery does not create a concurrent local execution.
- Result publication is independent of command ACK. Broker outage leaves a durable pending outbox event.
- NATS outage does not stop existing telemetry, reconciliation, or Collector supervision.

No Core HTTP API or schema is implied by these subjects. Production Core interoperability requires separate contract approval and implementation.
