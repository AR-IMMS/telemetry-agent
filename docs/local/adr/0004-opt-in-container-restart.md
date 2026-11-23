# ADR 0004: Opt-in Docker container restart through Operations

**Status:** Accepted by the user on 2026-11-15 for PR-09 Phase 2.

**Scope:** `container.restart` only; it does not authorize service restarts, logs, shell commands, or other mutations.

## Decision

- Core remains the business authorization authority. Operations commands arrive on the authenticated, installation/binding-scoped Core publisher subject. Agent-local checks are execution safety, not a replacement for Core IAM.
- Restart registration is disabled by default. An administrator must set the local process environment variable `AR_IMMS_OPERATIONS_CONTAINER_RESTART_ENABLED=true`; remote Core policy cannot set this flag.
- A target must independently opt in with the Docker label `ar-imms.operations.container-restart.enabled=true`.
- The request must name the full 64-hex Docker container ID in both target and parameters. Agent performs a fresh inspection and requires the returned ID to match, the target to remain opted in, and its state to be `running` before requesting a restart. A name is never used as a fallback.
- Agent issues one Docker Engine restart API call. It does not retry the call. If the result cannot be established, the result is `UNKNOWN` and the same operation ID returns that stored result without executing again.
- After restart, Agent verifies the same immutable ID using either an increased Docker `RestartCount` or a strictly newer `StartedAt` timestamp. Docker Engine may leave `RestartCount` unchanged for a manual restart, so start time is an accepted verification signal. `StartedAt` and restart counts are returned as evidence.
- A verified restart action is reported separately from target recovery. A container that restarted but is now stopped or unhealthy reports a successful action with `recovery_status=NOT_RUNNING` or `UNHEALTHY`.
- A `RUNNING` restart left by an Agent crash is finalized as public `UNKNOWN` / SQLite `UNKNOWN_OUTCOME`, with audit and result outbox records. Startup recovery never repeats the restart.
- The existing SQLite operation identity/fingerprint journal and result outbox remain authoritative. Same-ID delivery reuses its stored terminal result; conflicting fingerprints are rejected.

## Security boundary

The approved Docker endpoint is local (Linux Unix socket or Windows named pipe). Access to it may provide host-level privilege. The opt-in label narrows which containers the feature may act on but does not isolate the Agent's OS privilege or sandbox Docker itself. Deployments must grant Docker access only to an explicitly trusted Agent installation. The feature remains absent from runtime registration unless both the local flag and the target label are present.

## Consequences

- Read-only Operations, monitoring, telemetry delivery, Collector lifecycle, and enrollment remain independent of restart configuration and Docker action outcomes.
- Restart deadlines inherit the existing bounded Executor request deadline. The Docker Engine uses the container-configured stop timeout or its engine default; Agent does not request an immediate kill or indefinite wait.
- An unknown result requires a new, separately authorized operation after an operator has inspected the target; Agent never decides to repeat the side effect.
- Production NestJS Core interoperability remains separate. The local request/result schemas and Mock Core behavior do not claim a production Core endpoint implementation.

## Verification

- Unit tests cover local opt-in, target identity, per-container opt-in, non-running targets, one call, post-action evidence, unknown outcomes, and durable interrupted recovery.
- Native Windows and hosted Linux E2E use real Docker, pinned NATS JetStream, Mock Core, and a disposable labeled container. The live test verifies state and same-ID start-time change, then redelivers the operation with a distinct transport message ID and verifies no second restart.
