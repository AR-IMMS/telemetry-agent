---
name: commenting-code
description: Use when adding, reviewing, or improving code comments, especially around non-obvious behavior, constraints, workarounds, invariants, or design decisions.
---

# Commenting Code

## Core Principle

Comments should explain **why the code exists**, not narrate what the code already shows.

Prefer clear names and structure first. Add comments only when important intent, constraints, or reasoning would otherwise be lost.

## Rules

- Explain **why**, not obvious **what**.
- Document non-obvious constraints and invariants.
- Call out platform-specific behavior and workarounds.
- Explain abstractions that exist for testing or dependency injection.
- Comment important decision points, not every statement.
- Keep comments concise; usually 1–2 lines.
- Do not narrate control flow that is already obvious from the code.

## Example

Bad:

```go
// Retry rename when it fails.
```

Better:

```go
// Retry transient rename failures caused by short-lived file locks on Windows.
```

## Decision Rule

Before adding a comment, ask:

> If this comment is removed, would the reader lose important intent, rationale, a constraint, or a non-obvious assumption?

If not, prefer removing the comment.
