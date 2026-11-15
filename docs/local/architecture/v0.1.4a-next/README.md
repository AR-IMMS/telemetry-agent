# Architecture index — v0.1.4a-next

**Status:** proposed implementation baseline for review; not a declaration of
shipped functionality. This version is the proposed next target after review.
It does not erase historical design or implementation records. The supplied
proposal package is dated 2026-11-12.

**PR-09.5 consolidation set:** the merged integration source snapshot is
`b2599b8db31bfedfd75e360423d5aa98cfbcb5dc`. Use the
[source audit](consolidation/01-current-source-audit.md),
[context ownership map](consolidation/02-context-ownership.md),
[target architecture](consolidation/03-target-architecture.md),
[flexible source structure](consolidation/04-source-structure.md), and
[PR-09.5 migration plan](../../plans/v0.1.4a-next/pr-09.5-architecture-consolidation.md)
for source consolidation. These are planning documents awaiting architecture
approval; they do not authorize refactoring. The release
[architecture overview](architecture-overview.md) remains canonical for
product behavior and capability scope. The consolidation set is canonical for
the source audit and approved OD-01–OD-06 ownership decisions. Its physical
tree and migration sequence remain under architecture review. It supersedes
earlier source-to-target package guesses without deleting their history.

## Read order

1. [Release scope](../../requirements/v0.1.4a-next/release-scope.md) and
   [acceptance criteria](../../requirements/v0.1.4a-next/acceptance-criteria.md).
2. `architecture-overview.md`.
3. PR-09.5 [source audit](consolidation/01-current-source-audit.md),
   [context ownership](consolidation/02-context-ownership.md),
   [target architecture](consolidation/03-target-architecture.md), and
   [flexible source structure](consolidation/04-source-structure.md).
4. `context-boundaries.md` and the persistence/OTel/OpAMP contract docs.
   `source-structure.md` is historical and superseded for package placement;
   read it for provenance only, not as a migration map.
5. [Discovery and monitoring contracts](discovery-monitoring-contracts.md),
   including the approved PR-05 identity, opt-in, ownership, and evidence rules.
6. [Repository coding policy supplement](../../development/coding-policy.md).
7. [Complete plan index](../../plans/v0.1.4a-next/README.md),
   [migration roadmap](../../plans/v0.1.4a-next/migration-roadmap.md), and
   [PR-01 plan](../../plans/v0.1.4a-next/pr-01-foundation.md).
8. [Testbed specification](../../requirements/v0.1.4a-next/testbed-spec.md) and
   [test matrix](../../verification/v0.1.4a-next/test-matrix.md).
9. [Runbook readiness](../../runbooks/v0.1.4a-next/README.md).

## Decision status

The target favors OTel-first collection, native Windows/Linux operation,
application and host metrics, a discovery-provider boundary, official OTel
OpAMP Supervisor adoption, a Go OpAMP server adapter with a contract-compatible
Go Mock Core for the testbed, and incremental refactoring. PR-07 implementation
status is recorded in the checkpoint reports. OD-06 preserves already-approved
Core/local authority and revision semantics; unresolved control-plane policies
remain separate decisions. Production NestJS interoperability and pinned
component/platform follow-ups remain tracked. See [integration status](integration-status.md) and the
[cross-repository contract decision queue](../../requirements/v0.1.4a-next/cross-repository-contracts-open-decisions.md).

1. `architecture-overview.md`: consolidated target architecture and original 100-case coverage, amended with Nov 12 decisions and proposals.
2. `context-boundaries.md`: seven context owners; provider/listener abstraction and cross-context rules.
3. `source-structure.md`: historical Go package layout and migration hypotheses; superseded by `consolidation/04-source-structure.md`.
4. `state-and-event-model.md`: SQLite persistence/CAS, typed dispatcher, recovery.
5. `otel-integration-strategy.md`: OTel-first receivers/observer/receiver_creator and host/app/facility plans.
6. `opamp-integration.md`: revised OpAMP/Supervisor target, ownership transfer gates, contracts, and open authority/security.
7. `diagrams/`: one component diagram and six Mermaid flows; diagrams are proposed behavior, not executed traces.

Do not copy these into `docs/shared` until both Core and Agent owners review
contract changes. OpAMP Supervisor adoption and the Go Mock Core are proposals;
exact artifact compatibility, contract authority, and production NestJS Core
interoperability remain unverified.
