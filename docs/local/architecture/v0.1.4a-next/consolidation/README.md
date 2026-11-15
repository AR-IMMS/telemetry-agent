# PR-09.5 Consolidation Review Set

**Status:** all five PR-09.5 planning documents and OD-01 through OD-06 are
architecture-approved (2026-11-15). Implementation through PR-09.5g is
authorized under the migration plan's per-slice gates; PR-10 is excluded. The
audit is anchored to integration merge
`b2599b8db31bfedfd75e360423d5aa98cfbcb5dc`.

Read in this order:

1. [Current source audit](01-current-source-audit.md) — verified packages,
   imports, SQLite migrations/transactions, CLI and compatibility surfaces.
2. [Context ownership map](02-context-ownership.md) — existing source to the
   seven business contexts plus supporting boundaries and approved OD-01–OD-06 decisions.
3. [Target architecture](03-target-architecture.md) — approved contracts,
   dependencies, process boundaries and invariants.
4. [Flexible source structure](04-source-structure.md) — source-grounded
   capability tree, package disposition and documentation audit.
5. [PR-09.5 migration plan](../../../plans/v0.1.4a-next/pr-09.5-architecture-consolidation.md)
   — review gates, incremental slices, verification and rollback.

The [release architecture overview](../architecture-overview.md) remains
canonical for product behavior and acceptance scope. These documents are
canonical for source audit, approved OD-01–OD-06 ownership, target architecture,
and source structure.
Earlier design history is retained; superseded package guesses are labeled as
such. Remaining contracts are validated before their dependent slice under
the approved migration order. Stop after PR-09.5g; PR-10 remains excluded.
