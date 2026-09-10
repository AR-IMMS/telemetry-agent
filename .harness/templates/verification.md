# Verification: <Task Title>

## Context

- Plan: <link to plan>
- Commit / branch: <reference>
- Date: <YYYY-MM-DD>
- Verifier: <name or agent>

## Acceptance Criteria

| ID   | Criterion                | Evidence                                    | Result      |
| ---- | ------------------------ | ------------------------------------------- | ----------- |
| AC-1 | <observable requirement> | <test, command, log, screenshot, or output> | PASS / FAIL |
| AC-2 | <observable requirement> | <evidence>                                  | PASS / FAIL |

## Automated Checks

| Check                  | Scope / Command | Result      |
| ---------------------- | --------------- | ----------- |
| Tests                  | `<command>`     | PASS / FAIL |
| Lint / Format          | `<command>`     | PASS / FAIL |
| Typecheck              | `<command>`     | PASS / FAIL |
| Build                  | `<command>`     | PASS / FAIL |
| Integration / Contract | `<command>`     | PASS / FAIL |

## Manual Checks

- [ ] <manual scenario>
      - Expected: <expected result>
      - Actual: <actual result>

## Regression Checks

- [ ] Existing behavior remains unchanged: <evidence>
- [ ] Error and edge cases checked: <evidence>
- [ ] Public contracts remain compatible: <evidence or N/A>

## Scope Check

- [ ] No unrelated files changed.
- [ ] No generated files were edited incorrectly.
- [ ] No new duplicate abstraction was introduced.
- [ ] Documentation updated if required.

## Result

- Status: PASS / PASS WITH LIMITATIONS / FAIL
- Known limitations:
- Follow-up work:
