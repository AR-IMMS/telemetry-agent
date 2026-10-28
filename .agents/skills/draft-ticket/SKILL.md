---
name: draft-tickets
description: Use when a spec, plan, or conversation needs to be split into reviewable implementation tickets saved locally before publication.
disable-model-invocation: true
---

# Draft Tickets

Turn a spec, plan, or conversation into a local ticket draft for the user to review. This skill does not create or modify issues in an external tracker.

## Process

### 1. Gather context

Work from the current conversation. If the user provides a spec, issue, or pull request reference, read its full content and relevant comments.

Explore the codebase when needed to distinguish implemented behavior from planned behavior. Use the project's glossary and respect relevant ADRs.

### 2. Draft the tickets

Split the work into small, independently verifiable outcomes. Prefer a complete path through the layers that are in scope. A backend-only ticket does not need UI work.

For each ticket:

- Describe the user-visible or system behavior it delivers.
- Write acceptance criteria that can be checked.
- List only genuine blockers.
- Use project terminology.
- Avoid file paths and code snippets unless a prototype captures an important decision.
- Add a prefactoring ticket only when the implementation needs it.
- For a wide mechanical refactor, use an expand–migrate–contract sequence instead of forcing artificial vertical slices.

### 3. Save a local draft

Create one Markdown bundle per feature at:

`.scratch/ticket-drafts/<feature-slug>/tickets.md`

Use this format:

# Ticket draft: <Feature>

**Source:** <spec, issue, or conversation reference>
**Review status:** draft

## 1. <Ticket title>

**What to build:** <Outcome from the user's or system's perspective>

**Acceptance criteria**

- [ ] <Verifiable behavior>
- [ ] <Verifiable behavior>

**Blocked by:** None

## 2. <Ticket title>

**What to build:** <Outcome from the user's or system's perspective>

**Acceptance criteria**

- [ ] <Verifiable behavior>

**Blocked by:** Ticket 1

### 4. Review with the user

Present the proposed ticket list with titles, outcomes, and blockers. Ask whether any ticket should be split, merged, or changed.

Apply requested edits to the local draft. Keep its review status as `draft` until the user approves it. After approval, set it to `approved`.

Do not publish tickets or call tracker tools from this skill.
