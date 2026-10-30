# Desired State and Collector Configuration Lifecycle

## Purpose

This lifecycle lets the Agent change managed telemetry dependencies without
leaving the OpenTelemetry Collector with a stale configuration or removing a
resource that the Collector may still scrape.

It separates requested state, active configuration, and confirmed runtime
state.

## Generation Model

| Field                 | Meaning                                                                                                   |
| --------------------- | --------------------------------------------------------------------------------------------------------- |
| `DesiredGeneration`   | Latest dependency state requested by the operator.                                                        |
| `ActivatedGeneration` | A valid Collector configuration for that desired state was rendered, validated, and atomically activated. |
| `AppliedGeneration`   | The Collector reported ready while running the activated configuration.                                   |

A dependency receiver is rendered only when that dependency is enabled in
desired state.

## Lifecycle Overview

```mermaid
flowchart TD
    A["Operator command<br/>install / disable / uninstall"] --> B["Persist desired state"]

    B --> C{"Desired state changed?"}
    C -- "No" --> D["Return without new generation"]
    C -- "Yes" --> E["Increment DesiredGeneration"]

    E --> F["Render receivers from enabled dependencies"]
    F --> G["Validate candidate Collector config"]
    G -- "Invalid" --> H["Keep previous active config<br/>DesiredGeneration remains ahead"]
    G -- "Valid" --> I["Atomically activate config"]
    I --> J["Set ActivatedGeneration"]

    J --> K["Runtime watcher observes<br/>ActivatedGeneration changed"]
    K --> L["Stop current Collector"]
    L --> M["Start Collector with active config"]
    M --> N{"Collector ready?"}

    N -- "No" --> O["Keep pending state<br/>No host teardown"]
    N -- "Yes" --> P["Set AppliedGeneration"]

    P --> Q{"Pending teardown<br/>for applied generation?"}
    Q -- "No" --> R["Lifecycle complete"]
    Q -- "Yes" --> S["Verify Agent ownership"]
    S --> T["Execute dependency teardown"]
    T --> U["Complete teardown in state"]
    U --> R
```

## Disable and Uninstall Sequence

```mermaid
sequenceDiagram
    actor Operator
    participant CLI as agentctl
    participant State as State Store
    participant Config as Config Coordinator
    participant Collector as Collector Runtime
    participant Teardown as Dependency Teardown

    Operator->>CLI: dependency uninstall libre-hardware-monitor
    CLI->>State: Mark disabled, create pending uninstall
    State-->>CLI: DesiredGeneration = N + 1

    CLI->>Config: Render desired configuration
    Config->>Config: Validate candidate config

    alt Validation fails
        Config-->>CLI: Error
        Note over State,Config: Previous Collector config remains active
    else Validation succeeds
        Config->>Config: Atomically activate config
        Config->>State: ActivatedGeneration = N + 1
        Config-->>CLI: Uninstall scheduled

        Collector->>State: Poll active generation
        Collector->>Collector: Restart using active config
        Collector->>Collector: Wait for health endpoint

        alt Collector is not ready
            Note over Collector,Teardown: Pending teardown is retained
        else Collector is ready
            Collector->>State: AppliedGeneration = N + 1
            Collector->>Teardown: Uninstall owned resources
            Teardown->>Teardown: Stop/remove host resources
            Teardown->>State: Complete pending teardown
        end
    end
```

## Operations

| Command                       | Result                                                                                                                                               |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dependency install <name>`   | Installs or reconciles a supported dependency, records fresh-install ownership, enables its receiver, and activates the new Collector configuration. |
| `dependency disable <name>`   | Removes the receiver from desired configuration and schedules a non-destructive teardown after Collector readiness.                                  |
| `dependency uninstall <name>` | Removes the receiver from desired configuration and schedules physical cleanup after Collector readiness.                                            |
| `dependency pending`          | Lists teardown operations waiting for the matching Collector generation.                                                                             |
| `dependency status`           | Combines host inspection with persisted lifecycle state. Dependencies absent from both are shown as `not installed`.                                 |

## Safety Rules

- A candidate configuration is validated before it replaces the active one.
- `ActivatedGeneration` changes only after atomic configuration activation.
- The runtime watcher reacts to `ActivatedGeneration`, not directly to
  `DesiredGeneration`.
- Disable and uninstall remove the Collector receiver before host teardown.
- Teardown runs only after `AppliedGeneration` reaches the pending generation.
- Physical cleanup requires recorded Agent ownership. Existing or
  third-party resources are not automatically claimed or removed.
- A failed teardown remains pending instead of being marked complete.

## Failure and Recovery

If configuration validation fails, the previous valid Collector configuration
continues running. The requested generation remains ahead of the activated
generation, making the incomplete operation visible in persisted state.

If the Collector does not become ready, host resources are preserved. A later
runtime restart can apply the activated generation and continue the pending
teardown safely.

## Strengths

- Prevents the Collector from scraping a dependency after it is removed.
- Keeps desired state and actual runtime progress observable.
- Uses atomic configuration replacement.
- Limits destructive actions to resources the Agent recorded as its own.
- Supports safe recovery after interrupted operations.

## Current Limitations

- Teardowns run on the local machine only; there is no remote fleet control.
- A dependency installed before ownership tracking is not automatically
  adopted for physical removal.
- Node Exporter end-to-end validation requires a Linux host.
