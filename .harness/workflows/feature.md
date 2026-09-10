# Core Feature Delivery Workflow

This workflow is the default path for delivering a feature or a meaningful
change. Skills may extend a stage, but must not bypass the core gates.

## Core Flow

```text
Align
  -> Context
  -> Decision Gate
  -> Plan
  -> Implement
  -> Verify
  -> Review
  -> Finalize
```

## Invariants

- Every change has a clear objective and observable acceptance criteria.
- Every change has an explicit scope and out-of-scope list.
- Architectural decisions are recorded before implementation.
- Verification evidence is required before review.
- Review evaluates both the originating requirements and the code quality.
- A skill may add steps, but cannot silently remove a required gate.

## Stage 1: Align

### Goal

Turn the request into an agreed outcome.

### Actions

- Clarify ambiguous requirements and terminology.
- Define the user-visible or system-level outcome.
- Identify constraints, assumptions, and non-goals.
- Write acceptance criteria.

### Output

- Objective
- Acceptance criteria
- Scope and out-of-scope list

### Gate

Do not implement while the objective or acceptance criteria remain ambiguous.

## Stage 2: Context

### Goal

Load the smallest relevant set of project knowledge.

### Actions

- Read the applicable `AGENTS.md` files.
- Read relevant project context, architecture, and contracts.
- Inspect existing implementations, tests, and consumers.
- Record assumptions and unresolved questions.

### Output

- Relevant files and modules
- Existing contracts and constraints
- Context gaps

### Gate

Escalate when required context, ownership, or contracts cannot be identified.

## Stage 3: Decision Gate

### Goal

Decide whether the change requires an architectural decision.

Create an ADR before implementation when the change affects:

- architecture or module boundaries;
- public APIs, schemas, or events;
- data ownership or persistence;
- infrastructure or deployment topology;
- security or compatibility strategy;
- a technology or platform choice.

For local changes, explicitly record: `ADR: Not required`.

### Output

- Accepted ADR, or
- Explicitly documented decision that no ADR is required

## Stage 4: Plan

### Goal

Define the smallest coherent implementation path.

### Actions

- Identify affected files and contracts.
- Choose the implementation seam.
- Define test and verification strategy.
- Identify risks, migrations, and rollback needs.
- Split work when one change is too large to verify safely.

### Output

- `plan.md`

### Gate

The plan must be specific enough that another engineer can execute it.

## Stage 5: Implement

### Goal

Implement the planned behavior without expanding scope.

### Actions

- Follow the selected implementation skill when applicable.
- Prefer a vertical slice and small reversible changes.
- Add or update tests at the agreed seam.
- Preserve existing contracts unless the plan explicitly changes them.

### Optional Skills

- TDD for new behavior or regression fixes.
- Domain modeling for unclear terminology or boundaries.
- Research for external or unfamiliar technology.
- Prototyping for unresolved design questions.

### Gate

If implementation reveals a new architectural decision or invalidates the
plan, stop and update the ADR/plan before continuing.

## Stage 6: Verify

### Goal

Produce evidence that acceptance criteria are satisfied.

### Actions

- Run targeted tests first.
- Run affected-scope lint, typecheck, build, and integration checks.
- Run broader checks for shared, core, protocol, schema, or infrastructure changes.
- Check edge cases and failure paths.
- Record failures and limitations honestly.

### Output

- `verification.md`

### Gate

No review or completion claim without verification evidence.

## Stage 7: Review

### Goal

Evaluate the change independently from implementation.

### Actions

- Review the diff against the plan and acceptance criteria.
- Check architecture, boundaries, tests, security, and scope.
- Look for regressions, duplicate abstractions, and hidden coupling.
- Classify findings as blocker, major, or minor.

### Optional Skills

- Code review.
- Security review.
- Performance review.
- Domain-specific review.

### Output

- `review.md`

### Gate

Blockers and major findings must be resolved or explicitly accepted by the
responsible owner.

## Stage 8: Finalize

### Goal

Close the change with a traceable handoff.

### Actions

- Update project documentation when behavior or contracts changed.
- Update changelog only for user-visible or operationally relevant changes.
- Prepare the PR summary.
- Record known limitations and follow-up work.

### Output

- PR summary
- Changelog entry when applicable
- Completion summary

## Debugging Variant

For a bug or regression, use this flow inside the Implement stage:

```text
Reproduce
  -> Minimize
  -> Write regression test
  -> Form hypothesis
  -> Instrument
  -> Fix
  -> Re-run regression test
```

The bugfix still must pass Verify, Review, and Finalize.

## Skill Extension Rule

A skill must declare:

- which stage it extends;
- what input it consumes;
- what artifact or decision it produces;
- what core gate remains required afterward.

Skills are extensions of this workflow, not alternative workflows.
