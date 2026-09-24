# Managed Dependencies Runbook

## Inspect Available Dependencies

```powershell
agentctl dependency list
```

The result is filtered by the local operating system.

## Inspect Lifecycle State

```powershell
agentctl dependency status
```

Interpret the result as follows:

| Result                         | Meaning                                           | Operator action                                                   |
| ------------------------------ | ------------------------------------------------- | ----------------------------------------------------------------- |
| `enabled (healthy)`            | Runtime and managed configuration are healthy.    | No action.                                                        |
| `enabled (healthy, drifted)`   | Runtime works, but managed configuration differs. | Run the dependency install command to reconcile.                  |
| `enabled (unhealthy, drifted)` | Runtime failed and managed configuration differs. | Run the dependency install command; inspect logs if repair fails. |
| `enabled (unhealthy)`          | Runtime failed without detected managed drift.    | Inspect the dependency-specific runbook before changing it.       |
| `disabled (unknown)`           | Dependency is absent or disabled.                 | Install it if the host requires it.                               |
| `unknown (...)`                | Inspection could not complete.                    | Resolve the reported permission, platform, or system error.       |

## Install or Reconcile One Dependency

```powershell
agentctl dependency install windows-exporter
```

Use the stable name shown by `dependency list`.

## Select Multiple Dependencies Interactively

```powershell
agentctl dependency install
```

Controls:

```text
Up/Down     Move cursor
Space       Select or clear an item
Enter       Confirm selected items
q or Esc    Cancel
```

The selector shows at most ten items at once and scrolls as the cursor moves.

## Escalation Rules

- Run machine-wide Windows installation commands from an Administrator terminal.
- Do not manually replace a managed service, task, or configuration while an
  `agentctl` reconciliation is running.
- If reconciliation refuses to overwrite an unhealthy dependency, use the
  dependency-specific runbook to determine ownership and preserve diagnostics.
- Capture `agentctl dependency status` output and relevant service/task logs
  before escalating an incident.
