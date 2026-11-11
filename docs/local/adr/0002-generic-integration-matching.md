# ADR-0002: Generic Integration Matching and Configuration Candidate Boundary

- **Status:** Accepted for S4.3 implementation
- **Date:** 2026-11-09
- **Decision owner:** Not recorded
- **Related task / plan:** S4.3 Integration Matching & Configuration Resolution
- **Related design:** [S4.3 Technical Design](../architecture/s4.3-integration-matching.md)
- **Supersedes:** N/A
- **Superseded by:** N/A

## Context

S4.1/S4.2 provide provider-neutral inventory and an opt-in Docker provider. The Agent needs to identify applicable monitoring Integrations across containers, host services and future infrastructure resources without embedding technology-specific decisions in Discovery, S3 or M2. The existing S1 Registry and Resolver already own versioned definitions, instance validation and Collector-intent contribution. Matching must end before desired-state approval and activation.

## Decision drivers

- Support multiple resource types and Integration definitions without technology-specific branches in generic orchestration.
- Preserve Clean Architecture and S1/S2/S3/M2 ownership.
- Produce deterministic, explainable and safe candidates from a consistent inventory snapshot.
- Keep S4.3 read-only and leave proposal approval to S4.4.

## Decision

S4.3 uses a **hybrid minimal matcher**: exact declarative selectors handle common identification; an optional pure matching/candidate-building function on an existing Integration Definition handles cases selectors cannot express. The existing S1 Registry is extended to enumerate matchable definitions deterministically and to reject more than one matchable version for an Integration ID. Other exact versions remain available for existing desired instances.

The generic matcher consumes a read-only Integration-owned projection of one consistent inventory snapshot. It validates candidates with the existing Definition validation behavior, returns ordered evidence, required inputs, capability status and immutable candidate settings/digest, and performs no persistence or external calls. Composition adapts Discovery inventory and assesses capabilities through a narrow read-only port backed by the S2 Catalog. Integration does not depend on Discovery, SQLite or Collector Runtime.

Docker inventory adds bounded image reference/repository evidence with exact matching and backward-compatible JSON decoding. Endpoint selection is conservative: only a unique explicit loopback host-published endpoint may be auto-resolved initially. Wildcard, remote, private-container and ambiguous endpoints require input. Credentials are never inferred from labels or environment variables.

S4.3 ends at matching results and diagnostics. S4.4 owns persistent proposal identity/deduplication; approval belongs to Configuration and must re-read inventory and re-match before desired-state CAS. S3/S2/M2 continue to resolve approved instances, compile, activate and recover as before.

## Bounded-context ownership

- **Discovery & Inventory:** observations, provider identity, normalized evidence, freshness and snapshots. The future S4.4 Proposal Manager may consume S4.3 results but is outside this decision.
- **Integration:** definitions, Registry, generic matcher, definition-owned selectors/builders and candidate diagnostics.
- **Configuration:** future S4.4 approval policy and existing Desired Integration state/CAS.
- **S3/S2/M2:** existing reconciliation, Collector compilation/capability catalog, lifecycle activation/readiness/journal/LKG.
- **Composition:** read adapter, S2 capability wiring and read-only CLI diagnostics only; no matching policy or S4.3 state ownership.

## Alternatives considered

1. **Technology branches in Discovery coordinator:** rejected. It makes inventory aware of Integration policy and requires central edits for every technology.
2. **Independent Integration matcher registry:** rejected. It duplicates S1 definition identity/version registration and creates drift between matching and instance resolution.
3. **Full Datadog-style template engine and check scheduler:** rejected. It exceeds the PoC need and duplicates lifecycle scheduling; this Agent delegates collection to the OTel Collector.
4. **Matcher embedded in Discovery:** rejected. It couples the provider-neutral inventory context to Integration definitions and candidate validation.
5. **Declarative selectors only:** not selected as the sole mechanism. Exact selectors cover ordinary cases, while an optional Definition-owned pure builder supports technology-specific endpoint/configuration evidence without a general rules DSL.

## Consequences

### Positive

- New Integrations add Definition-owned selector/builder behavior and composition registration without changing generic matching, S3 or M2.
- Candidates are deterministic, explainable and suitable for S4.4 proposal deduplication/stale approval checks.
- Existing manually configured Definitions and exact-version instances continue to work.
- Matching is testable without Docker, SQLite or Collector activation; the live E2E proves only the real inventory-to-candidate path.

### Limitations and migration impact

- Existing Docker rows lack image evidence and remain unmatched until a later successful snapshot populates additive fields.
- A resource fingerprint is needed because inventory exposes a global revision, not per-resource revisions.
- No candidate proves network reachability, Integration health or telemetry delivery.
- Image alias and endpoint policy must stay conservative to avoid false matches.
- No SQL migration was required for additive resource JSON; existing and new records must remain backward compatible.

## Security

- Keep bounded normalized fields and current sensitive-attribute filtering; never persist environment variables or credentials.
- Reject wildcard bind endpoints as destinations; do not probe networks.
- Keep capability assessment separate from identification and backed by pinned artifact evidence. Unsupported/unverified results are not actionable by default.

## Compatibility

- Matching metadata remains optional on existing S1 definitions; manual exact-version instance resolution continues to use the existing Registry, validation and Resolver.
- Docker image evidence is an additive resource JSON extension. Missing fields in older records remain valid and simply provide no image-based match evidence.
- S4.3 does not change Desired Integration persistence, S3 reconciliation, S2 compilation, M2 activation or the SQLite schema.

## Deferred decisions

- Generic device ResourceKind, device providers and PDU/cooling integrations (AD-06 deferred).
- Network reachability policy beyond explicit loopback host-published endpoints.
- Complex matching rules beyond exact selectors and optional pure Definition behavior.
- S4.4 durable proposal lifecycle, duplicate suppression against desired state, approval and desired-state mutation are separate from this decision and are covered by the S4.4 design.
- Additional production integrations beyond Redis proof.
