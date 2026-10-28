# M1 — Implementation Plan

## S1 — Architecture Foundation & Boundaries

**Goal:** Establish the foundation for the new architecture without breaking current functionality.

**Scope:**

- Define bounded contexts, interfaces, and dependency rules.
- Separate the Lifecycle Host from business orchestration.
- Establish the composition root and the minimum required extension points.
- Preserve the existing CLI, exporters, and Collector runtime.

**Acceptance:** The Agent can still install, start, and stop, and collect host metrics as before. No circular dependencies are introduced.

## S2 — Reconciliation & Configuration Activation

**Goal:** Replace tightly coupled orchestration with a reconciliation-driven lifecycle.

**Scope:**

- Add Desired State, Applied State, and revision tracking.
- Have the Reconciler produce an immutable configuration plan.
- Add a Collector Configuration Compiler and validation.
- Add an Activation Coordinator to manage apply, verify, and commit.
- Handle repeated reconciliation and stale revisions.

**Acceptance:** Configuration changes are applied through reconciliation; unchanged configuration does not restart the Collector; invalid configuration does not replace the active configuration.

## S3 — SQLite Persistence & Restart Recovery

**Goal:** Keep Agent state consistent and recoverable after restart.

**Scope:**

- Use one SQLite `state.db` database with WAL enabled.
- Persist desired/applied revisions and activation records.
- Preserve the Last-Known-Good configuration.
- Recover after Agent or Collector restart and failed activation.
- Migrate from the current local state while preserving ownership and teardown behavior.

**Acceptance:** After an Agent restart, the Collector configuration and host telemetry are restored. A failed activation does not lose the committed configuration.

## Dependencies and Delivery

```mermaid
flowchart LR
    S1["S1: Foundation"] --> S2["S2: Reconciliation"]
    S2 --> S3["S3: Persistence & Recovery"]
    S3 --> DONE["M1 Complete"]
```

| Slice | Primary deliverable |
|---|---|
| S1 | Architecture boundaries with existing functionality preserved |
| S2 | Working reconciliation and activation lifecycle |
| S3 | Durable state and verified restart recovery |

### Solo Development Principles

- Each slice should be a small milestone or a large, independently reviewable pull request.
- Do not build the full Integration SDK in M1; define only the contracts needed by the next steps.
- Do not refactor unrelated modules solely to improve their structure.
- Run regression tests throughout all three slices instead of deferring them until the end.

## M1 Completion

M1 is complete when the Agent's existing functionality runs through the new architecture, the configuration lifecycle is managed through reconciliation, and the Agent can recover after restart.
