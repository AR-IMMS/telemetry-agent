# Architecture index — v0.1.4a-next

**Status:** canonical documentation for release target `v0.1.4a-next`. PR-01
through PR-09.5f are merged; PR-09.5g final verification is in progress. This
index distinguishes implemented checkpoints from target behavior and remaining
release gates while preserving the historical design record.

**PR-09.5 consolidation set:** the merged integration source snapshot is
`b2599b8db31bfedfd75e360423d5aa98cfbcb5dc`. Use the
[source audit](consolidation/01-current-source-audit.md),
[context ownership map](consolidation/02-context-ownership.md),
[target architecture](consolidation/03-target-architecture.md),
[flexible source structure](consolidation/04-source-structure.md), and
[PR-09.5 migration plan](../../plans/v0.1.4a-next/pr-09.5-architecture-consolidation.md)
for source consolidation. The user approved the complete consolidation set on
2026-11-15 and authorized implementation through PR-09.5g; PR-10 remains
excluded. The release
[architecture overview](architecture-overview.md) remains canonical for
product behavior and capability scope. The consolidation set is canonical for
the source audit, approved OD-01–OD-06 ownership decisions, target architecture
and source structure. Its physical tree is flexible implementation guidance.
It supersedes
earlier source-to-target package guesses without deleting their history.

The integration head after PR-09.5f is
`5a06d7d6fe8ac6c2ba09043ad1fd6f3a769e1ce8` (PR #38; integration CI
[43834547036](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43834547036)
passed). PR-09.5g executable source is at
`3c87b8e1882f1beffa4a07a1eb610822cfbe5200`; PR #39 implementation/workflow head
`0a90b9252d99981fd837a85f03e48f29d1eff227` passed Windows/Ubuntu CI
[43839603999](https://github.com/AR-IMMS/telemetry-agent/actions/runs/43839603999).
PR #39 targets `chore/v0.1.4a-next`; checkpoint merge is pending. PR-10 has not
started.

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

## Diagrams and source direction

- [Component diagram](diagrams/component.mmd) shows process boundaries and
  mutually exclusive Collector ownership. New installs require the managed
  Supervisor endpoint; existing installations preserve their stored owner
  until explicit migration, with legacy ownership available for rollback.
- [Context collaboration diagram](diagrams/context-dependencies.mmd) shows
  business and data collaboration. Its arrows are not Go imports.
- [Go import dependency view](consolidation/03-target-architecture.md#code-dependency-go-imports)
  defines the separate allowed import direction. Collaboration arrows do not
  imply package dependencies.

## Browse by context

Use the ownership map for responsibility and contracts, the source structure
for current-to-target package disposition, and the architecture overview for
behavior and acceptance scope. Package destinations are flexible guidance;
this index does not duplicate the package tree.

| Context | Ownership and contracts | Source disposition | Behavior and acceptance |
|---|---|---|---|
| Identity | [Ownership map](consolidation/02-context-ownership.md) | [Identity](consolidation/04-source-structure.md#identity) | [Architecture overview](architecture-overview.md) |
| Inventory | [Ownership map](consolidation/02-context-ownership.md) | [Inventory](consolidation/04-source-structure.md#inventory) | [Architecture overview](architecture-overview.md) |
| Monitoring | [Ownership map](consolidation/02-context-ownership.md) | [Monitoring](consolidation/04-source-structure.md#monitoring) | [Architecture overview](architecture-overview.md) |
| Configuration | [Ownership map](consolidation/02-context-ownership.md) | [Configuration](consolidation/04-source-structure.md#configuration) | [Architecture overview](architecture-overview.md) |
| Runtime | [Ownership map](consolidation/02-context-ownership.md) | [Runtime](consolidation/04-source-structure.md#runtime) | [Architecture overview](architecture-overview.md) |
| Health | [Ownership map](consolidation/02-context-ownership.md) | [Health](consolidation/04-source-structure.md#health) | [Architecture overview](architecture-overview.md) |
| Operations | [Ownership map](consolidation/02-context-ownership.md) | [Operations](consolidation/04-source-structure.md#operations) | [Architecture overview](architecture-overview.md) |

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
7. `diagrams/`: current component ownership, context collaboration, and six
   protocol/operational flows. Go imports are shown separately in the
   consolidation target architecture.

Do not copy these into `docs/shared` until both Core and Agent owners review
contract changes. PR-07 is PASS for Agent/Mock Core acceptance; production
NestJS Core interoperability remains separately unverified. Earlier OpAMP and
Supervisor notes remain design history; current acceptance status and known
limits are recorded in the PR-07 and PR-09.5 reports.
