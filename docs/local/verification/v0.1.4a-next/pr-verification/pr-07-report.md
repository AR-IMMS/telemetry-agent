# PR-07 readiness report — OpAMP Core/Agent integration

**Status:** BLOCKED — implementation requires joint Core/Agent contracts before protocol or persistence changes.
**Release target:** `v0.1.4a-next`
**Branch:** `feat/pr-07-opamp`
**Agent baseline:** `af2f5db4c6b8832bc7bcc87d332d899dad01b452` (PR-06 merge)
**Core baseline checked:** `dev` at `9420437b532f667375736027df8f02024e3e9e0d`

## Verified current state

- Agent source search found no OpAMP client, OpAMP protocol status types, or remote-config transport. Existing effective-config digests and activation journal record local Collector candidates and outcomes; they do not establish remote configuration authority or OpAMP status semantics.
- Core `dev` contains Agent enrollment, credential/binding lifecycle, and read-only registry capabilities. The Core Agent Management README explicitly says Agent Control APIs are not implemented and does not claim heartbeat, telemetry ingestion, or Collector activation. The repository tree has no OpAMP, remote-config, Agent-config, or control-plane module.
- Core source inspected: [Agent Management README at the checked commit](https://github.com/AR-IMMS/core-backend/blob/9420437b532f667375736027df8f02024e3e9e0d/src/modules/agent-management/README.md).
- The project’s local OpAMP plan requires an authority ADR, explicit Agent association, protocol/schema pinning, actual runtime status mapping, and Core IAM/audit checks before its implementation steps.
- The official [OpAMP Go library](https://github.com/open-telemetry/opamp-go) supplies client transport and callbacks for remote configuration and protocol status. It does not decide AR-IMMS Core-versus-local policy authority, revision conflict semantics, credential binding, or which Collector runtime outcomes map to an application’s durable desired/effective state.

## Contracts requiring joint approval

1. **Remote policy authority:** fields Core may set; local bootstrap retention/overrides; whether updates are full snapshots or patches; deletion semantics; target enforcement; stale, equal, malformed, and conflicting revision behavior; canonical hash inputs.
2. **Identity and revocation:** binding from credential to installation and registered Node; whether/how OpAMP `instance_uid` is persisted or rotated; rebinding behavior; credential revocation and offline semantics. `instance_uid` is not an authentication credential.
3. **Authentication and transport:** HTTP polling versus another OpAMP transport; TLS requirements, credential scope/rotation, redirect/proxy policy, server association, message-size/rate limits, and replay handling. Current docs describe both “plain HTTP polling” and authenticated TLS; reconcile that wording before implementation.
4. **Configuration status:** map native OpAMP remote-config states to Agent receipt, validation, persistence, candidate activation, readiness, rollback, and effective-config reporting. Decide what survives Agent restart and what Core retains. `APPLIED` must not be asserted before the agreed runtime success boundary.
5. **Remote secrets and schema:** allowed monitoring-policy fields, secret references and delivery scope, schema/content-type versioning, and rejection behavior. Do not serialize credentials or accept arbitrary Collector YAML/expressions.
6. **Core API/security ownership:** Core endpoint/authentication/authorization, Admin policy authoring and audit requirements, isolated test namespace, and the separate Core repository implementation/review scope.

## Open cross-repository contradictions and dependencies

- `docs/local/architecture/v0.1.4a-next/opamp-integration.md` labels the transport “authenticated plain-HTTP polling” but describes TLS in its flow; the PR-07 plan says “HTTPS plain HTTP polling.” Decide whether this means the OpAMP HTTP transport over HTTPS and specify the authentication mechanism.
- The target flow assumes a versioned policy document inside OpAMP `config_map`, but no approved schema, target/hash/revision contract, or local-config conflict policy exists yet.
- The accepted enrollment API and registry are not evidence that a Core OpAMP server or control API exists. Implementing only the Agent half would not satisfy the planned cross-repository acceptance gates.
- `docs/local/requirements/v0.1.4a-next/cross-repository-contracts-open-decisions.md` is a local decision queue and is not published to `docs/shared/`; no shared-submodule change was made.

## Work and verification

- Read-only checks: current Agent branch/baseline, PR-07 plan and architecture docs, Agent symbol search, Core `dev` repository metadata/tree, and Core Agent Management README.
- No production code, config, schema, database migration, Core source, or shared documentation changed for PR-07.
- OpAMP protocol implementation tests are **NOT RUN** because required application contracts and Core server implementation are absent.

## Exit conditions

PR-07 can start after Core and Agent owners approve the six contract groups above, reconcile transport/TLS wording, decide how those artifacts are reviewed/published, and authorize the separate Core repository change. Keep the PR-07 checkpoint BLOCKED until those prerequisites exist. PR-08 depends on the resulting status/reporting contract; PR-09 remains independently blocked on its security ADR.
