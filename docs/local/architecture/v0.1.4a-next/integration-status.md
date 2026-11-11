# v0.1.4a-next documentation integration status

**Status:** documentation integrated for review. The architecture and release
documents remain proposals; this integration does not approve implementation
or claim that acceptance evidence exists.

## Migration summary

- Added the complete versioned architecture set and diagrams under
  `docs/local/architecture/v0.1.4a-next/`.
- Added the repository coding-policy supplement under `docs/local/development/`.
- Added the versioned migration roadmap and PR-01 plan under
  `docs/local/plans/v0.1.4a-next/`; the complete conditional PR-01–PR-10 plan
  set is now indexed there.
- Added release scope, acceptance criteria, testbed, and cross-repository
  contract decision queue under `docs/local/requirements/v0.1.4a-next/`.
- Added the verification matrix under `docs/local/verification/v0.1.4a-next/`
  and runbook readiness notes under `docs/local/runbooks/v0.1.4a-next/`.
- Updated `docs/README.md` and `docs/ARCHITECTURE.md` to distinguish the
  current implementation description from the proposed next target.
- Kept `docs/local/` and `docs/shared/` in place. No files in the shared
  submodule were edited. The proposed cross-repository items remain local
  review decisions and are not published contracts.

## Supersession and history

The accepted v0.1.4 architecture document is retained as historical design
input and is no longer the proposed next-version target. The earlier
`target-agent-architecture.md` already identifies itself as superseded by
v0.1.4; it remains historical analysis. The v0.1.4a-next architecture overview
is now the proposal to review, while `docs/ARCHITECTURE.md` continues to
describe the implemented PoC baseline. Existing verification reports remain
historical evidence for their stated scope and do not satisfy the new release
matrix.

## Reconciliations and open decisions

| Topic | Reconciliation / current status |
|---|---|
| Proposed versus implemented | All v0.1.4a-next architecture, plan, requirements, and test documents are proposed/planned. No proposed feature is represented as shipped or verified. |
| Version label | The integrated package calls the target `v0.1.4a-next`, while `architecture-overview.md` labels its design `0.2-draft`. Confirm the canonical release/version label before implementation plans are approved. |
| Current versus target architecture | Existing v0.1.4 and implementation documents are preserved. Entry points identify them as historical/current-state references, respectively; v0.1.4a-next is the proposed next target. |
| Coding policy | The local supplement extends the shared policy. File-size guidance now preserves the shared 300–400-line general range and justification requirement; an optional 450-line automated ceiling is not permission to routinely exceed that guidance. Repo-specific Go documentation rules remain in force. |
| Shared contracts | OpAMP policy schema, remote operations contract, installation-to-OpAMP identity mapping, and revocation remain a local publication queue. Core and Agent review is still required; `docs/shared/` was not modified. |
| Configuration authority | Core-versus-local precedence, ownership, deletion, revision/hash semantics, and conflict handling remain open and require an ADR before broad remote rollout. |
| OTel component support | Receiver/observer component availability and behavior must be validated against the exact pinned Collector build and each target OS. The proposal is not capability evidence. |
| Receiver ownership migration | Legacy Agent materialization must not overlap with Observer/Receiver Creator for the same target. Switch only after platform parity evidence and an explicit migration gate. |
| Operations security | Mutating remote operations remain disabled/deferred until authorization, privilege, identity, idempotency, audit, timeout, and unknown-outcome behavior are enforceable and reviewed. |
| Plan execution boundaries | The plans are conditional, not implementation authorization. PR-07 includes Core NestJS work and requires coordinated review/change in `core-backend`; this repository's plan does not authorize changing that repository. PR-05 and PR-09 retain explicit go/no-go gates. |
| File-size guidance in plans | The ≤450-line automated gate is supplemental. The shared policy's usual 300–400-line range and justification requirement still apply. |
| Runbooks | The new v0.1.4a-next runbook document records readiness requirements. Final install/upgrade/rollback/uninstall procedures depend on approved CLI and service contracts and must be verified on native Windows and Linux. Existing runbooks remain for current behavior. |
| Evidence | The new matrix is `NOT RUN`/`PLANNED` as written. Existing test reports are not promoted to release evidence without matching platform, artifact, and scenario proof. |

## Files changed by this integration

New source package documents were mapped without flattening versioned content:

- `docs/local/architecture/v0.1.4a-next/`: architecture overview, 5 companion
  documents, index, and 7 Mermaid diagrams; this integration status file is new.
- `docs/local/development/coding-policy.md`.
- `docs/local/plans/v0.1.4a-next/`: roadmap, refreshed plan index, existing
  PR-01 plan, and new PR-02 through PR-10 plans.
- `docs/local/requirements/v0.1.4a-next/`: release scope, acceptance criteria,
  testbed, and contract decision queue.
- `docs/local/verification/v0.1.4a-next/test-matrix.md`.
- `docs/local/runbooks/v0.1.4a-next/README.md`.
- `docs/README.md` and `docs/ARCHITECTURE.md`.

## Validation

The subsequent complete-plan package added PR-02 through PR-10 and refreshed
the plan index and roadmap. All nine new plan files match the supplied package
by SHA-256. Markdown relative file/directory links checked: 51; broken: 0.
`git diff --check` passed. No production code, configuration, migration, or
runtime behavior was changed.
