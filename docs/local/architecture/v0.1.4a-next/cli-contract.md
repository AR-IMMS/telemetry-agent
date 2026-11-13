# `agentctl` CLI Contract — v0.1.4a-next

**Status:** PROPOSED / REVIEW REQUIRED<br>
**Target release:** `v0.1.4a-next`<br>
**Scope:** Engineer-facing CLI UX, backward compatibility, command ownership, PR implementation mapping.<br>
**Source of current commands:** repository command inventory supplied for review; exact flags and JSON output must be verified against source HEAD before implementation.

> This document defines target CLI behavior, **not shipped functionality**. Do not change implementation PR plans solely to match this file. All commands below labeled **NEW** are proposals.

## 1. UX principles

1. Primary journey: **Install → Enroll → Monitor → Diagnose → Configure → Operate**.
2. Default workflow hides Collector YAML, receiver matching internals, and SQLite tables. Advanced commands remain available.
3. Keep existing commands/flags/output and `--state-path` semantics compatible during migration; use aliases/warnings before removal.
4. Commands that mutate state report actual committed/applied results, never assume activation or OTLP delivery from request acceptance.
5. Linux `systemd` and Windows SCM use the same public engineer-facing CLI where possible; OS-specific service internals are documented explicitly.
6. Never expose enrollment secrets, OpAMP credentials, tokens, or unredacted diagnostic secrets in human or JSON output.

## 2. Current CLI baseline (existing)

```text
agentctl bootstrap
agentctl dependency
agentctl integration
agentctl run
agentctl inventory
agentctl discovery
agentctl status
agentctl enroll
agentctl service         # Windows dispatcher entrypoint; missing in root help
agentctl install
agentctl uninstall
agentctl help

agentctl dependency {list,install,status,pending,configure,disable,uninstall}
agentctl integration {configure,update,enable,disable,delete,list,inspect,status,history,retry,match}
agentctl integration catalog {list,inspect}
agentctl integration template {create,update,list,inspect,enable,disable,delete}
agentctl inventory {list,inspect,status}
agentctl discovery proposals {list,inspect,dismiss,approve}
```

**Existing concurrency contracts:** integration/template writes use revision safeguards (`--expected-revision` where applicable); proposal approval requires `--revision` and `--expected-desired-revision`. Preserve these while old commands remain operational. Verify precise option applicability against source.

## 3. Target CLI hierarchy

Markers: **KEEP** = existing; **EXTEND** = existing command with new behavior/subcommands; **NEW** = not shipped; **LEGACY** = retained for compatibility, not the default DX.

```text
agentctl
├── install                         KEEP
├── uninstall                       KEEP
├── enroll                          KEEP
├── run                             KEEP, internal service entry
├── status                          EXTEND: Agent / Collector / OpAMP / health
├── doctor                          NEW: actionable local diagnostics
├── inventory                       EXTEND
│   ├── list | inspect | status      KEEP
│   └── refresh                     NEW: request fresh observation cycle
├── monitoring                      NEW
│   ├── list | inspect | explain
│   ├── profile
│   │   ├── list | inspect
│   │   ├── validate
│   │   └── apply
│   └── reconcile
├── config                          NEW
│   ├── show | validate | history
│   └── sync
├── operations                      NEW, opt-in and gated
│   ├── inspect-container
│   ├── container-logs
│   ├── restart-container
│   └── status
├── dependency                      KEEP
├── integration                     LEGACY / advanced manual control
├── discovery                       LEGACY proposals, not default flow
├── bootstrap                       KEEP / advanced
├── service                         KEEP / Windows dispatcher internal
└── help                            EXTEND
```

**Scope warning:** `monitoring profile apply` is a request to update a supported local profile, **not** permission to bypass an unresolved Core/local configuration-authority policy. Its final write semantics must be decided before shipping. Similarly, `config sync` is an explicit sync/reconnect attempt and must never imply “Core always wins.”

## 4. Commands, observable behavior and context ownership

| Command | Context | Expected result / boundary |
|---|---|---|
| `install`, `uninstall`, `bootstrap` | Runtime & Installation | Native service and Collector/dependency lifecycle; preserve ownership-safe uninstall. |
| `enroll` | Identity & Connectivity | Enroll/resume with Core, persist protected credential; never print secrets. |
| `run`, `service` | App/Runtime | Long-running host execution; `service` is currently Windows-only internal SCM entry. |
| `status` | Health & Reporting | Aggregate local installation, Collector readiness, active generation, OpAMP status, exporter/monitoring evidence. |
| `doctor` **NEW** | Health & Reporting | Report actionable checks and evidence; diagnostic errors do not mutate monitored infrastructure. |
| `inventory refresh` **NEW** | Resource Inventory | Trigger one provider reconciliation cycle; distinguish provider errors from missing resources. |
| `monitoring list/inspect/explain` **NEW** | Monitoring + Health | Map resource → profile/rule → receiver capability → applied generation → scrape/export evidence; `unknown` when not measurable. |
| `monitoring profile ...` **NEW** | Configuration + Monitoring | List/inspect/validate/update trusted profiles; use OTel-native capabilities, not custom scraping code. |
| `monitoring reconcile` **NEW** | Configuration | Request immediate evaluation of persisted desired state; no-op must not restart Collector. |
| `config show/validate/history/sync` **NEW** | Configuration + Identity | Show local/remote/effective/applied fingerprints and OpAMP sync state without leaking secrets. |
| `operations ...` **NEW** | Controlled Operations | Opt-in Docker diagnostics, later authorization-gated restart; exact target, bounded output, durable execution result. |
| `dependency ...` | Runtime & Installation | Keep Windows Exporter, LHM/LHM Exporter, optional Node Exporter support. |
| `integration ...`, `discovery proposals ...` | Compatibility facade | Existing manual/proposal workflows remain callable until a separately approved migration/deprecation. |

## 5. Example engineer journeys (proposed)

```bash
# 1. Install and enroll (actual flags are retained from the current CLI)
agentctl install --config-root <path>
agentctl enroll --core-origin https://core.example
agentctl status
agentctl doctor

# 2. Understand an application that is not monitored
agentctl inventory refresh
agentctl inventory list --active-only
agentctl monitoring explain --resource <resource-id>

# 3. Extend monitoring for a custom Python /metrics application
agentctl monitoring profile validate --file ./python-api.yaml
agentctl monitoring profile apply --file ./python-api.yaml
agentctl monitoring reconcile
agentctl monitoring list

# 4. Inspect central config and request a sync
agentctl config show
agentctl config history
agentctl config sync
agentctl status

# 5. Diagnose a container; optional mutating operation in a later approved stage
agentctl operations inspect-container --id <container-id>
agentctl operations container-logs --id <container-id> --tail 100
agentctl operations restart-container --id <container-id> --operation-id <uuid>
```

The examples describe **target UX**, not verified executable syntax for new subcommands.

## 6. Status/doctor semantics

`status` is a concise summary. `doctor` explains *why*, and `monitoring explain` diagnoses a specific resource. They must not duplicate entire outputs. PR-08 adds `doctor` and extends the existing local `/v1/status` response with an additive `assessment` object; the existing top-level fields remain for compatibility.

Suggested stable categories (exact machine-readable schema pending review):

- Agent lifecycle: `RUNNING | STOPPED | DEGRADED | UNKNOWN`.
- Core/OpAMP: `CONNECTED | DISCONNECTED | NOT_CONFIGURED | UNKNOWN`.
- Collector: `READY | NOT_READY | RECOVERING | FAILED | UNKNOWN`.
- Config: `RECEIVED | VALIDATING | APPLYING | APPLIED | FAILED | UNKNOWN` as **AR-IMMS presentation**; map accurately to the actual OpAMP protocol status instead of claiming all are OpAMP enum values.
- Monitoring path: `DISCOVERED | ELIGIBLE | CONFIGURED | APPLIED | SCRAPING | EXPORTING | DELIVERY_VERIFIED` are **distinct evidence stages**, not a single boolean.

The local `/v1/status` `assessment` contains `state`, `generatedAt`, and an ordered `components` list. Each component reports `component`, `state`, `freshness`, `completeness`, optional `observedAt`, optional `validUntil`, and a bounded `reasonCode`. Component states are `healthy`, `degraded`, `unknown`, or `unsupported`; freshness is `fresh`, `stale`, or `unknown`; completeness is `complete`, `partial`, or `unknown`. A stale result requires an evidence owner to supply both an observation time and a validity deadline; no global age threshold is inferred. Missing evidence remains `unknown`.

The `agent_runtime=healthy` observation means the local status snapshot was generated. The existing top-level `status` remains the Collector/configuration runtime projection. `status` prints a compact component summary; `doctor` prints the component evidence, timestamps, reason codes, and existing Collector endpoint failure records. An explicit Collector scrape failure marks collection evidence degraded with unknown freshness; absence of a failure is not positive collection evidence. Neither command reports collection or backend delivery as healthy without the corresponding evidence. Core health-report transmission is not implemented because no approved Core reporting contract exists. The CLI does not add JSON output in this checkpoint.

**Non-equivalence:** Collector readiness ≠ successful scrape ≠ OTLP delivery. Agent must report `UNVERIFIED` if backend receipt was not measured.

## 7. Compatibility, deprecation and automation

- **PR-03 must not remove** old `integration` or `discovery proposals` commands, nor change their revision/CAS behavior.
- Preserve existing flags, exit codes, JSON key meaning, environment/state paths and automation-compatible output. Capture command and output golden tests before modifications.
- Fix discoverability of existing Windows `service` command in help, but label it Windows/internal; on Linux return a clear unsupported error rather than pretending SCM is portable. If Linux service management UX is later introduced, design it separately.
- New commands must support structured output (`--output json` **proposed**, not a confirmed existing option); define an explicit schema, stable error code and redaction contract before release.
- Old commands become **legacy/advanced** only when replacement behavior is verified. Deprecation messaging should include replacement and supported release; actual removal is **out of scope for v0.1.4a-next**.
- Preserve `--state-path` compatibility and existing concurrent-change error semantics. New aliases must call the same application use cases; avoid two independently maintained implementations.
- Do not repurpose `agentctl service` for generic OS service restarts: these are separate controlled operations.

## 8. Remote Operations security and edge cases

`operations restart-container` must **not ship** until the remote/local trust model is approved, and the executor enforces authorization beyond a UI check. Local administrative access and Core-authenticated remote access are distinct trust paths.

Minimum requirements: exact immutable target/container identity, preflight validation, expiry/authorization, opt-in enablement, audit, durable idempotency result, bounded timeout/output, and never retry ambiguous mutations blindly. Stale target, Docker unavailable, expired command, duplicate request, Agent restart midway, and successful restart without workload recovery must be test cases.

`operations inspect-container` and `container-logs` must avoid exposing credentials from container environment and bound log/output size and retention.

## 9. PR implementation mapping (no edits to PR documents required)

| PR | CLI implementation responsibilities | Required gate |
|---|---|---|
| **PR-01 — Foundation** | Adopt this contract; capture current CLI help, options, exit-code and output snapshots. | Baseline is reproducible; no runtime change. |
| **PR-02 — Events/SQLite** | No CLI redesign; preserve `--state-path` and committed-state behavior. | Existing CLI regression tests pass. |
| **PR-03 — App Composition** | Thin command adapters; root help fixes, especially `service`. | Old command behavior and output remain compatible. |
| **PR-04 — Context Migration** | CLI delegates to context-owned public use cases; no new UX required. | Old `dependency`, `integration`, `discovery`, `inventory` behavior passes. |
| **PR-05 — OTel Discovery** | Add `inventory refresh`; expose observer/provider diagnostics as needed. | No ghost resource/receiver after convergence; no unnecessary Collector reactivation. |
| **PR-06 — Monitoring Profiles** | Add `monitoring list/inspect/explain/profile/reconcile`; keep legacy manual commands. | Redis, PostgreSQL, Python `/metrics` real-data E2E; profile validation and no-rebuild scenario. |
| **PR-07 — OpAMP/Supervisor** | Review whether `config show/validate/history/sync` and `status` need updates after Supervisor capability/parity; preserve public CLI until an explicit contract is approved. | 07a pinned capability; 07b Go server/Mock Core contracts; 07c single-owner runtime adoption, offline/replay/no-op/LKG and actual OTLP verified. |
| **PR-08 — Health Reporting** | Add `doctor`; expand `status` and monitoring evidence explanations. | Honest readiness/scrape/export state and redaction tests. |
| **PR-09 — Controlled Operations** | Add `operations inspect-container/container-logs/status`; `restart-container` only after security gate. | Auth, stale-target, duplicate ID, timeout/unknown result, audit. |
| **PR-10 — Release Hardening** | Complete CLI docs, native Windows/Linux smoke, help and JSON/exit-code golden tests. | New and old CLI regression + real E2E passing with recorded evidence. |

## 10. Acceptance criteria for this CLI contract

- New commands exist only when their owning context supports corresponding use cases; no fake success placeholders.
- Default journey has no forced manual discovery proposal approval and no hand-editing of generated OTel YAML.
- Built-in dependency commands remain usable and ownership-safe.
- `doctor`/`monitoring explain` diagnose missing capability, unreachable endpoint, missing credentials, invalid policy, rejected generation, Collector error, OpAMP offline, and unverified delivery.
- Linux/Windows platform differences are explicit; release reports use PASS/FAIL/BLOCKED/UNSUPPORTED with evidence.
- Existing command compatibility is preserved or explicitly approved with migration documentation.

## 11. Open decisions (not delegated to coding agent)

1. Core-vs-local profile authority and allowed actions for `monitoring profile apply`.
2. Exact config payload schema, OpAMP status mapping and `config sync` behavior.
3. Machine-readable output schema, flags and stable exit codes for new commands.
4. Local versus remote authorization model for mutating operations.
5. Whether `inventory refresh` should refresh Agent inventory only, OTel observer config, or both; default proposal is **Agent inventory only**, since OTel owns its own observation lifecycle.

**Related docs:** [Architecture overview](architecture-overview.md), [Context boundaries](context-boundaries.md), [Source structure](source-structure.md), [Migration roadmap](../../plans/v0.1.4a-next/migration-roadmap.md).
