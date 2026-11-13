# Architecture index — v0.1.4a-next

**Status:** proposed implementation baseline for review; not a declaration of
shipped functionality. This version is the proposed next target after review.
It does not erase historical design or implementation records. The supplied
proposal package is dated 2026-11-12.

## Read order

1. [Release scope](../../requirements/v0.1.4a-next/release-scope.md) and
   [acceptance criteria](../../requirements/v0.1.4a-next/acceptance-criteria.md).
2. `architecture-overview.md`.
3. `context-boundaries.md`, `source-structure.md`, `state-and-event-model.md`,
   `otel-integration-strategy.md`, and `opamp-integration.md`.
4. [Discovery and monitoring contracts](discovery-monitoring-contracts.md),
   including the approved PR-05 identity, opt-in, ownership, and evidence rules.
5. [Repository coding policy supplement](../../development/coding-policy.md).
6. [Complete plan index](../../plans/v0.1.4a-next/README.md),
   [migration roadmap](../../plans/v0.1.4a-next/migration-roadmap.md), and
   [PR-01 plan](../../plans/v0.1.4a-next/pr-01-foundation.md).
7. [Testbed specification](../../requirements/v0.1.4a-next/testbed-spec.md) and
   [test matrix](../../verification/v0.1.4a-next/test-matrix.md).
8. [Runbook readiness](../../runbooks/v0.1.4a-next/README.md).

## Decision status

The proposal favors OTel-first collection, native Windows/Linux operation,
application and host metrics, a discovery-provider boundary, official OTel
OpAMP Supervisor adoption after capability/parity checks, a Go OpAMP server
adapter with a contract-compatible Go Mock Core for the immediate testbed, and
incremental refactoring. Production Core interoperability and decisions remain open where the source
documents say so, including pinned Collector component/platform validation,
Core versus local configuration authority, and remote-operation privilege and
transport. See [integration status](integration-status.md) and the
[cross-repository contract decision queue](../../requirements/v0.1.4a-next/cross-repository-contracts-open-decisions.md).

1. `architecture-overview.md`: consolidated target architecture and original 100-case coverage, amended with Nov 12 decisions and proposals.
2. `context-boundaries.md`: seven context owners; provider/listener abstraction and cross-context rules.
3. `source-structure.md`: proposed Go package layout, current migration map and import guardrails.
4. `state-and-event-model.md`: SQLite persistence/CAS, typed dispatcher, recovery.
5. `otel-integration-strategy.md`: OTel-first receivers/observer/receiver_creator and host/app/facility plans.
6. `opamp-integration.md`: revised OpAMP/Supervisor target, ownership transfer gates, contracts, and open authority/security.
7. `diagrams/`: one component diagram and six Mermaid flows; diagrams are proposed behavior, not executed traces.

Do not copy these into `docs/shared` until both Core and Agent owners review
contract changes. OpAMP Supervisor adoption and the Go Mock Core are proposals;
exact artifact compatibility, contract authority, and production NestJS Core
interoperability remain unverified.
