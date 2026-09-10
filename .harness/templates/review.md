# Review: <Task Title>

## Context

- Plan: <link to plan>
- Verification: <link to verification>
- Diff / PR: <link or reference>
- Reviewer: <name or agent>
- Date: <YYYY-MM-DD>

## Review Scope

- Reviewed files:
- Reviewed contracts:
- Related ADR: <link or N/A>

## Findings

### [BLOCKER] <Finding title>

- Location: `<file:line or module>`
- Problem: <what is wrong>
- Impact: <why it matters>
- Recommendation: <specific fix>

### [MAJOR] <Finding title>

- Location:
- Problem:
- Impact:
- Recommendation:

### [MINOR] <Finding title>

- Location:
- Problem:
- Recommendation:

<!-- Remove unused severity sections. -->

## Review Checklist

### Correctness

- [ ] Acceptance criteria are satisfied.
- [ ] Edge cases and failure paths are handled.
- [ ] Existing behavior is not unintentionally broken.

### Architecture

- [ ] Module and domain boundaries are preserved.
- [ ] Existing abstractions are reused appropriately.
- [ ] No unnecessary coupling or duplicate abstraction was introduced.
- [ ] Public contracts are compatible or explicitly changed.

### Testing

- [ ] Tests cover observable behavior.
- [ ] Tests are not implementation-detail assertions.
- [ ] Verification evidence is sufficient.

### Maintainability

- [ ] The change is appropriately scoped.
- [ ] Naming and structure are understandable.
- [ ] Documentation is updated where necessary.

### Security and Operations

- [ ] No secrets or sensitive data are exposed.
- [ ] Errors and logs are safe and useful.
- [ ] Configuration and deployment impact are understood.

## Decision

- Status: APPROVE | REQUEST_CHANGES | APPROVE_WITH_FOLLOW_UP
- Blocking findings:
- Required follow-up:
- Reviewer summary:
