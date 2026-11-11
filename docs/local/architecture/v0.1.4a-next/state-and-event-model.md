# SQLite authority, events and lifecycle state

## Storage ownership

One local SQLite WAL database remains authoritative. `platform/sqlite` owns open/close, transactions, migration dispatch and lock primitives, not domain tables. Each context owns its repository SQL and revision semantics. Preserve atomic approval+desired state transitions during compatibility migration: do not split into two writes and rely on events.

## Typed advisory events

| Event | After commit by | Subscribers |
|---|---|---|
| `InventorySnapshotCommitted` | Inventory after complete durable revision | Capability projection, config materialization wakeup |
| `MonitoringPolicyChanged` | Configuration policy revision | Config reconciliation |
| `DesiredConfigurationChanged` | Configuration desired revision | Runtime convergence wakeup |
| `CollectorGenerationApplied` | Runtime terminal activation journal | Health/reporting |
| `CollectorGenerationFailed` | Runtime recorded failure/recovery | Health/reporting |

Events carry IDs/revision/fingerprint only, never credentials, full config or mutable object graphs. No-op commits emit no change events. Dispatcher: bounded queues, stable subscription order, startup/stop, nonreentrant publish, timeout/full error, panic isolation and idempotent handlers. **Publish only after commit, outside locks.** Existing startup and periodic SQLite reconciliation remain and repair missed wakeups. No external broker/outbox in MVP.

## Lifecycle semantics

- Config stages: received → validated/persisted → pending apply → applied-ready OR failed/restored. Native OpAMP statuses are separate from richer AR-IMMS projections.
- Collector generation and candidate journal track validation, activation, readiness and LKG. Ready does **not** assert OTLP delivery.
- Resource provider snapshot error is not equivalent to resource deletion. Complete successful missing observations control presence transitions.
- Mutating operations (if approved) require durable records and idempotency; **never** use the in-memory event dispatcher as a command queue.

## Failure evidence

Test concurrent CLI/service writes, process crash after SQLite commit before event enqueue, bounded-queue overflow, late/duplicate events, reopen, full startup recovery, failed Collector validation and process restart; compare before/after SQLite fixtures.
