# Template-driven Automatic Monitoring — Implementation Plan

> **For agentic workers:** Execute this plan natively in this session, one phase at a time. Keep unrelated worktree changes unstaged and untouched.

**Goal:** Implement locally authorized Integration Configuration Templates that automatically materialize deterministic, template-owned desired Integration instances from valid resource matches and converge them through the existing S3/S2/M2 path.

**Architecture:** Integration owns the constrained template and matching contracts, while Configuration owns template authority, persistence, and generated-instance materialization. Discovery supplies consistent inventory evidence and diagnostics. The matcher evaluates all enabled templates before any projection so processing order cannot select a winner. SQLite applies a complete projection with CAS in one short transaction; S3 remains the only desired-to-applied controller and M2 remains the only activation/recovery lifecycle.

**Tech stack:** Go, existing Integration Registry/Matcher/Resolver, Discovery inventory and provider, SQLite WAL/migrations, existing Agent Service, S3 reconciliation, S2 compiler, M2 lifecycle, `agentctl` conventions, Docker Desktop Redis and isolated OTLP sink.

**Spec:** `D:\Study\FPTU\FPTU\WDP301\projects\telemetry-agent\docs\local\architecture\telemetry-agent-template-autodiscovery-goal-en.md` (read-only source for AC-01…AC-17 and Groups A/B/C).

## Global Constraints

- Keep the four implementation parts, AC-01…AC-17, and Groups A/B/C from the accepted specification unchanged.
- Local template enablement is the sole initial template-scoped authorization; a shipped Integration Definition does not grant activation authority.
- Templates use constrained selectors, defaults and allowlisted resource-field bindings; no expression language, arbitrary Collector YAML, dynamic plugins, second matcher, scheduler, policy engine, or activation path.
- Preserve existing Go candidate builders, Definition validation, Collector-neutral contributions, proposal/manual approval, S3 reconciliation and M2 activation/recovery.
- Evaluate every enabled applicable template for a resource/Integration pairing before materialization. Conflicts fail closed; retain owned records and emit diagnostics.
- Never mutate generated desired instances from failed providers, incomplete snapshots, or transient evaluation/persistence failures.
- Apply complete materialization batches atomically with template/inventory/desired-state CAS; unchanged projections must not advance desired-set revisions.
- Generated logical identity is `(owner scope, Template ID, provider, provider instance, resource ID, Integration ID)`; Template revision is mutable metadata, never an identity input. A Template targets one Integration definition, so its Integration ID is fixed for that Template ID.
- Definition defaults declare whether they are overridable. Authorized Template settings may override only defaults explicitly marked overridable. Resource bindings use explicit per-setting precedence and cannot replace fixed operator settings implicitly.
- Template disable is reversible; logical delete is a retained tombstone that prevents Template ID reuse and reactivation. Both suspend only that Template's generated instances while preserving provenance/history.
- On a CAS conflict, reread the latest Inventory, Template, and Desired State snapshots and recompute the entire projection before retrying.
- Keep manual and differently owned instances immutable to template projection; non-test Go files stay ≤450 LOC.
- Do not modify `target-architecture-v0.1.4.md` or `docs/shared` absent an explicitly reviewed contradiction.

## Review Focus

- Two templates conflict for one resource/Integration while a generated instance is active: suspend the generated instance without deleting its ownership record, and report the conflict; never pick by order.
- Projection is interrupted after evaluation: transaction rollback leaves the prior complete desired set intact; restart safely retries the same snapshot.
- Provider error or incomplete inventory never looks like resource disappearance; confirmed absence uses the existing two-complete-snapshot policy.
- Manual/differently-owned ID or binding collisions never overwrite the existing instance.
- Repeated identical discovery/template evaluation produces a no-op desired mutation and no Collector restart.

---

## Phase 1 — Integration Templates & Configuration Ownership

**Files:**
- Create: `internal/integration/template.go` — bounded template model, selectors, defaults, allowlisted field bindings, validation and immutable resolved settings.
- Create: `internal/configuration/template.go` — template CRUD and snapshot contracts, owner/source semantics, template revisions and local operator authorization.
- Create: `internal/agentstate/sqlite_template_schema.go` — additive migration for template persistence and template-set writer revision.
- Create: `internal/agentstate/sqlite_template.go` — SQLite template repository, consistent snapshots, CAS, restart and no-op semantics.
- Modify: `internal/integration/integration.go` — template-generated source/provenance and optional template identity/revision on instances, preserving legacy local/discovery decoding.
- Test: `internal/integration/*template*_test.go`, `internal/configuration/*template*_test.go`, `internal/agentstate/sqlite_template_test.go`.

**Interfaces:**
- Template owner/source is explicit (`local` initially; preserve a Core-owned value for later contract integration without adding Core behavior).
- Template identity is stable; template revision is distinct from desired-set and per-instance revisions.
- Template setting bindings read only validated resource identity, bounded allowlisted attributes, and a uniquely selected endpoint; conflicting or missing bindings fail closed.
- Generated instances record owner scope, template ID/revision, resource binding and a deterministic ID derived from the full logical identity tuple; template revision changes update metadata/configuration without changing identity.
- A Template permanently targets one Integration ID; changing a definition version does not change generated identity. Retired Template IDs remain tombstoned and cannot be reused.

- [x] Add domain tests for template validation, allowlist enforcement, secret rejection, stable identity and owner/provenance semantics.
- [x] Add Configuration contract tests for create/update/enable/disable/delete CAS and no-op revisions.
- [x] Implement an additive SQLite migration preserving existing state and template snapshots across restart.
- [x] Test concurrent template writes, migration from the current schema, idempotency, rollback and unchanged template operations.

## Phase 2 — Automatic Matching & Instance Materialization

**Files:**
- Create: `internal/integration/template_matching.go` — extend the existing Matcher with template-aware evaluation; globally group results by resource/Integration identity and produce deterministic candidate/diagnostic results.
- Create: `internal/configuration/materialization.go` — Configuration-owned batch projection contract and application use case.
- Create: `internal/agentstate/sqlite_materialization.go` — atomic SQLite projector with template, inventory-write, and desired-set CAS checks and ownership-limited upsert/suspension.
- Modify: `internal/integration/matching.go` and tests — combine template settings with existing Go builder output; reject incompatible overlapping settings and preserve current manual matching behavior.
- Test: template matcher, materializer contract, and SQLite atomicity/concurrency tests.

**Interfaces:**
- Matcher stays read-only and resolves the entire enabled-template set before returning any auto-materialization candidates.
- Definition defaults are applied first; explicit Template settings override only Definition defaults marked overridable. Resource bindings resolve according to a declared per-setting precedence. A binding cannot replace fixed Template/operator settings; conflicting sources fail closed. The existing Go builder can fill missing values, but differing overlap is an error. The existing Definition validator and secret checks remain mandatory.
- One projection batch is bound to a consistent inventory write revision, template-set write revision, and desired-set revision. On CAS conflict, the application rereads all three snapshots and recomputes before bounded retry; it never replays stale computed mutations.
- A deterministic conflict suspends only generated instances in the conflicting template/resource/Integration pairing and preserves their records. Manual or foreign-owned collisions are never modified.
- Missing/invalid/unsupported results become safe diagnostics/proposals; transient failures abort projection and preserve previous instances.

- [x] Test multiple matches, duplicate templates, existing active generated instances, manual collisions, unsupported capabilities, missing inputs and invalid settings.
- [x] Test the required conflicting-template scenario with no order-dependent winner and retained ownership records.
- [x] Test a failed/interrupted projection leaves every previous instance and the desired-set revision unchanged.
- [x] Test changed settings update only template-owned instances; identical settings are a transactional no-op.
- [x] Test concurrent desired/template/inventory writes produce a CAS conflict and a safe later retry.

## Phase 3 — Resource Lifecycle & Recovery

**Files:**
- Modify: `internal/discovery/coordinator.go` or add a small cycle callback adapter — invoke projection only after a complete committed inventory snapshot; keep provider failure behavior unchanged.
- Modify: `cmd/agentctl/discovery_runtime.go` — compose inventory, existing matcher, template snapshot and Configuration materializer in the existing Agent Service loop; no new scheduler.
- Modify: `internal/agentstate/sqlite_materialization.go` — reconcile owned instances across template disable/delete, explicit stopped state, confirmed resource absence and reappearance.
- Modify: `cmd/agentctl/main.go` / integration command dispatch — register thin template commands.
- Create: `cmd/agentctl/integration_template_command.go` — create, edit/update, list, inspect, enable, disable and delete local templates through Configuration use cases.
- Test: runtime cycle, CLI, recovery and full-suite compatibility.

**Interfaces:**
- Template disable suspends its generated instances but remains reversible; re-enable may restore matching instances with the same stable IDs. Template delete is a logical tombstone: retain history, suspend its instances, reject reuse/re-enable of that Template ID, and do not silently reactivate deleted-template instances.
- Confirmed resource stop/absence suspends only instances owned by the affected template/resource; retain records. Reappearance under an active, non-deleted Template re-enables the same deterministic instance.
- Provider failures, incomplete snapshots, stale evidence and transient matching/projection errors never suspend or delete generated instances.
- The current inventory policy (two complete misses) determines confirmed removal; explicit current stopped/non-running evidence may suspend immediately.
- Existing proposal refresh remains available for manual approval and coverage gaps; valid authorized template results do not require per-resource approval.
- SQLite polling remains the S3 cross-process desired-state detector; runtime change notifications are optional optimizations only.

- [ ] Test stop/remove/recreate, template disable/re-enable/delete, daemon outage, Agent restart, and transient projection errors against persisted instances.
- [x] Test CLI operations and ensure they do not invoke reconciliation directly.
- [x] Verify existing proposal approval behavior remains unchanged for manual/untemplated candidates.
- [ ] Verify S3 sees only actual desired-set changes and that no-op projection does not activate/restart the Collector.

## Phase 4 — Real Infrastructure E2E & Architecture Hardening

**Files:**
- Modify: `cmd/agentctl/discovery_live_e2e_test.go` or add a focused template E2E test — isolated Docker Redis containers, state DB and OTLP sink.
- Modify: `docs/local/architecture/s4.3-integration-matching.md` and `docs/local/architecture/s4.4-monitoring-proposal-approval-design.md` — document template authorization/materialization and preserve the manual proposal path.
- Modify: `docs/local/runbooks/monitoring-proposals.md` — add template lifecycle operations and diagnostics.
- Create/update: `docs/local/verification/template-autodiscovery-verification.md` — sanitized AC/test evidence, platform limitations and reproduction commands.
- Modify only relevant package tests and migration compatibility fixtures; no Target Architecture or shared-doc edits by default.

- [ ] Group A — Automatic Redis Monitoring:
  - Test 1 — Auto-monitor Redis: Create an enabled Redis Template and start a Redis Docker container. Expected: Agent discovers → matches → generates instance → activates Collector → OTLP sink receives Redis metrics. No manual approval is used.
  - Test 2 — One Template, multiple containers: Start 3 Redis containers on different ports. Expected: One Template produces 3 instances with different IDs. Collector has 3 Redis receiver instances. No duplicates appear after multiple discovery cycles.
  - Test 3 — Template update: Change the collection interval or another valid setting. Expected: All 3 instances are updated; Collector reconciles the correct new desired set without losing the hostmetrics pipeline.
- [ ] Group B — Lifecycle & Failure Scenarios:
  - Template disabled: Disable the Redis Template. Expected: Only that Template's generated Redis instances are disabled.
  - Template deleted: Delete the Redis Template. Expected: Only instances owned by that Template are cleaned up.
  - Redis stopped: Stop one container. Expected: The corresponding instance is handled according to the resource lifecycle policy.
  - Redis removed: Remove a container. Expected: After confirmed absence, the corresponding instance is removed/suspended.
  - Redis recreated: Create a new container using the same image. Expected: Identity is handled correctly; the new container is not confused with the old one.
  - Docker unavailable: Temporarily disconnect the Docker Engine. Expected: No mass deletion of instances.
  - Agent restart: Restart the Agent Service. Expected: Correct Templates and generated instances are restored.
  - Invalid template: Provide an invalid endpoint/interval configuration. Expected: Reject it and preserve the valid desired state.
  - Conflicting templates: Two Templates match the same resource. Expected: No conflicting instances are created automatically.
  - Unsupported capability: A Template requires an unsupported receiver. Expected: Do not activate; produce diagnostics.
- [ ] Group C — Heterogeneous Infrastructure:
  - Redis 7: Full monitoring E2E.
  - PostgreSQL 17: Discovery → Template matching → Needs Input/diagnostics.
  - MongoDB 8: Discovery → Template matching → Needs Input/diagnostics.
  - Unknown Docker image: Inventory contains a record, but no monitoring instance is created.
  - Redis + PostgreSQL + MongoDB concurrently: No cross-matching, no duplicates, and no Collector disruption.
- [ ] Run native Windows and Linux tests where available, full Go tests, race tests, vet, build, formatting and scoped diff checks; distinguish native runtime E2E from cross-platform builds.
- [ ] Map every AC-01…AC-17 and every Group A/B/C test to `PASS`, `FAIL`, `NOT RUN`, or `BLOCKED`; do not claim Collector readiness as telemetry delivery.
- [ ] For Group A Test 2, independently assert that Redis telemetry attributable to each of the three distinct Redis instances reaches the isolated OTLP sink; a running Collector or three configured receivers is insufficient.

## Acceptance mapping (verbatim IDs; source wording unchanged)

| ID | Acceptance criteria |
|---|---|
| AC-01 | An operator creates a Redis Template; the Agent automatically monitors a matching Redis container without approving each instance |
| AC-02 | One Template creates multiple instances with stable identities |
| AC-03 | Repeated discovery/matching does not create duplicates or unnecessary Collector restarts |
| AC-04 | A Template update changes only instances owned by that Template |
| AC-05 | Disabling/deleting a Template disables or removes only its generated instances |
| AC-06 | Manual Integration Instances are not overwritten or deleted |
| AC-07 | Missing endpoints, credentials, ambiguity, or unsupported capabilities → no automatic activation |
| AC-08 | Conflicting Templates → fail closed and produce diagnostics |
| AC-09 | A Docker daemon outage does not cause mass deletion of monitoring instances |
| AC-10 | A confirmed resource removal → its generated instance is handled according to the resource lifecycle policy |
| AC-11 | Restarting the Agent → Inventory, Templates, and desired instances are restored |
| AC-12 | The existing proposal/manual approval flow still works |
| AC-13 | All desired-state changes pass through the existing Configuration Reconciliation and Collector Lifecycle |
| AC-14 | Template revision, desired revision, and applied generation are kept distinct |
| AC-15 | Generated instances record their source Template, resource identity, and ownership |
| AC-16 | Local Templates do not falsely claim to be Core-managed |
| AC-17 | No plaintext secrets appear in Templates, Inventory, diagnostics, or logs |

**Delivery:** Four reviewable implementation phases, with focused commits per phase. Draft PR and CI publication occur only after the explicit GitHub token check requested by the user; never merge automatically.
