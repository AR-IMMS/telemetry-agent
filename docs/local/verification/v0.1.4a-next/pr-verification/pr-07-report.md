# PR-07 verification report — OpAMP Supervisor and control plane

**Status:** IN_PROGRESS — revised target and evidence gates documented; PR-07a
capability tests have not been run in this update. Do not merge or transfer
Collector lifecycle ownership on this evidence.
**Release target:** `v0.1.4a-next`
**Current branch at report update:** `feat/pr-07-opamp`
**Current documentation base:** `3d30268` plus the scoped documentation changes
listed in the release progress tracker; implementation baseline is PR-06 merge
`af2f5db4c6b8832bc7bcc87d332d899dad01b452`.

## Target change reviewed

The new proposal supersedes the earlier NestJS OpAMP adapter / Agent Go client
plan. It proposes three gated slices: (07a) exact-version OTel OpAMP Supervisor
capability spike without ownership transfer; (07b) reviewed Core/Agent contracts
plus Go OpAMP server and Go Mock Core; (07c) controlled runtime ownership
transfer and native platform E2E after parity. The immediate proposed Core policy
scope is only a versioned OTLP Gateway endpoint. Production Core changes and
shared-contract publication remain separate approval gates.

## Verified documentation and current-state facts

- Agent source/docs identify existing `internal/supervisor` and
  `internal/agentlifecycle` as the current Collector process/runtime path. They
  are distinct from the proposed OTel OpAMP Supervisor.
- Agent-repository filename search did not find the referenced
  `agent-enrollment.openapi.yaml` or Core API Baseline. Those authoritative
  artifacts must be located and reviewed before adding contracts; do not infer
  their schemas from the pasted proposal.
- The previous read-only readiness check inspected Core `dev` at
  `9420437b532f667375736027df8f02024e3e9e0d`; its Agent Management README said
  Agent Control APIs were not implemented. That is historical evidence, not a
  fresh current-Core audit or proof about the proposed Mock Core.
- Current official Collector Contrib Supervisor documentation describes
  server endpoints with `ws`, `wss`, `http`, and `https`; remote config is
  opt-in, is merged with an optional local config, written to the Collector
  config, and causes a Collector restart. Automatic remote-config rollback is
  configurable and disabled by default, and the described trigger is Collector
  failure to start. Reference: [OpAMP Supervisor specification](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/opampsupervisor/specification).
  These are upstream documentation findings, not pinned v0.160.0 artifact
  verification. Validate actual behavior and broader LKG guarantees in 07a.

## Contract and compatibility gates still open

1. Locate authoritative enrollment OpenAPI and Core API Baseline; confirm
   existing request/response, error, credential, revocation, cache and
   request-ID semantics.
2. Define installation/binding ↔ OpAMP `instance_uid` identity and secure
   Supervisor credential provisioning, storage, rotation, revocation and
   session invalidation. `instance_uid` is not authentication.
3. Define schema/version, target, desired/applied revision, hash canonicalization,
   local/remote field ownership, merge order, deletion and stale/conflict rules
   for the proposed Gateway endpoint policy.
4. Map protocol/runtime evidence to truthful received, validated, applying,
   applied, failed, recovered and unknown statuses. Readiness is not scrape or
   OTLP delivery evidence; an older applied hash after rollback is not success
   for a newer desired revision.
5. Establish exact Supervisor artifact existence/provenance/SHA, Windows/Linux
   support, compatibility with Collector Contrib v0.160.0, config composition,
   local updates, offline fallback and rollback behavior.
6. Review the migration/rollback design so the Agent Runtime remains sole
   lifecycle owner until a single reversible transfer after parity.

## Verification log

| Gate | Status | Evidence / note |
|---|---|---|
| Proposal-to-doc reconciliation | PASS | Architecture, plan index, migration roadmap, release scope, acceptance, test matrix, diagrams, report and progress tracker updated. |
| Exact Windows Supervisor artifact and checksum | NOT RUN | No artifact downloaded or verified in this documentation update. |
| Exact Linux Supervisor artifact and checksum | NOT RUN | No native Linux artifact/runtime check in this documentation update. |
| Collector v0.160.0 compatibility | NOT RUN | No binary interoperability check run. |
| Local+remote config composition / local update semantics | NOT RUN | Upstream docs describe optional local merge, config write and Collector restart; pinned artifact behavior remains unknown. |
| Startup fallback, offline/reconnect, Supervisor/Collector crash | NOT RUN | No test run. |
| Candidate failure and rollback/LKG | NOT RUN | Upstream rollback is opt-in and documented for startup failure; Agent LKG parity is unverified. |
| Actual OTLP delivery | NOT RUN | No endpoint-change test or sink evidence. |
| Windows native E2E | NOT RUN | No PR-07a live run. |
| Native Linux E2E | NOT RUN | No PR-07a live run. |
| Mock Core contract interoperability | NOT RUN | Not implemented or tested. |
| Production NestJS Core interoperability | BLOCKED | Separate Core implementation/contract approval gate; not implied by mock results. |
| Lifecycle ownership transfer / rollback | BLOCKED | Must wait for 07a parity and 07b contract gates; no transfer occurred. |

No production code, runtime config, schema, database migration, Core source, or
`docs/shared` submodule was changed as part of the documentation update. No
PR-07 implementation, capability, or E2E test was run. The pre-existing
untracked `cmd/agentctl/otel.yaml.lifecycle.lock` remains preserved and excluded.

## Merge audit requirement

Before opening/merging an implementation checkpoint, replace `NOT RUN` only with
reproducible evidence tied to exact commit and platform. Audit CI after the
final code, docs, and verification-log changes; include raw evidence locations,
artifact hashes, commands, environment versions, failure-injection results,
single-owner/rollback result, and actual OTLP sink evidence. Keep mandatory
gates open and checkpoint draft while any are unresolved.
