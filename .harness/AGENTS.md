# Reusable Agent Instructions

This file defines reusable engineering rules for AI coding agents.
Project-specific instructions take precedence when explicitly documented.

## 1. Read Context First

Before changing code, inspect:

1. Root `AGENTS.md`
2. `docs/PRODUCT.md`
3. `docs/ARCHITECTURE.md`
4. `docs/CONTRACTS.md`, when relevant
5. The nearest nested `AGENTS.md`

**Do not guess project structure, commands, contracts, or conventions.**

## 2. Understand the Task

Before implementation:

- Restate the requested outcome.
- Identify the affected files, modules, and contracts.
- Define observable acceptance criteria.
- Check existing implementations and patterns.
- Keep the change within the requested scope.

Ask for clarification when requirements, ownership, or expected behavior are ambiguous.

## 3. Plan Before Coding

For non-trivial work, create a short plan covering:

- intended behavior
- affected components
- risks and boundary changes
- verification steps

Prefer the smallest coherent implementation.

## 4. Preserve Boundaries

- Keep business logic inside its owning module.
- Do not bypass established APIs, interfaces, or events.
- Do not add business logic to shared utilities.
- Treat public APIs, schemas, events, and shared modules as protected boundaries.
- Do not refactor unrelated code.

## 5. Reuse Before Adding

Before creating a helper, wrapper, adapter, or abstraction:

1. Search the repository for an existing implementation.
2. Reuse or extend it when appropriate.
3. Avoid duplicating responsibility or creating parallel abstractions.

## 6. Source of Truth

- Do not edit generated files directly.
- Modify the source definition or generator.
- Regenerate affected outputs.
- Keep source and generated artifacts consistent.

## 7. Verification

Use repository-defined commands from the root `AGENTS.md` or project documentation.

At minimum, run the narrowest relevant checks:

- tests for changed behavior
- lint or formatting checks
- typecheck or build checks when applicable

Run broader checks when changing shared, core, protocol, schema, or infrastructure code.

Never claim completion without verification evidence.

## 8. Tests

Tests should protect observable contracts:

- behavior
- output shape
- state transitions
- error mapping
- compatibility
- regression cases

Prefer contract-level tests over implementation-detail tests.

## 9. Documentation

Update project documentation when changing:

- public behavior
- architecture or boundaries
- APIs, schemas, or events
- operational procedures
- configuration requirements

Do not put project-specific knowledge in this file.

## 10. Completion Report

Before finishing, report:

- what changed
- affected scope
- checks executed
- check results
- known limitations or follow-up work