# v0.1.4a-next — implementation and refactoring roadmap

**Status:** proposed release plan, PR boundaries subject to review. Every PR requires own implementation-plan document and verification evidence. Do not start behavioral changes until relevant design/security decision is approved.

| PR | Objective | Must not change | Verification gate |
|---|---|---|---|
| 01 | Docs, architecture import rules, code/comment policy, baseline fixtures | Runtime semantics | Architecture checks, existing tests/CLI snapshots |
| 02 | Extract SQLite technical layer and add typed event dispatcher | DB tables, CAS, polling | Migration + crash-after-commit/event-loss/concurrency tests |
| 03 | App composition root, thin CLI | CLI commands/output and runtime sequencing | CLI contract, install/service baseline |
| 04 | Split contexts and repositories by true owners | Enrollment/desired state/approval atomicity | Context repo tests, Windows/Linux builds |
| 05 | OTel Observer/Receiver Creator **spike** and gated delegation | Redis path until parity; no dual receiver owners | Native Linux/Windows Redis, multi-instance/recreate/no-ghost/OTLP |
| 06 | Generic application monitoring profiles, provisioning, mock facility extension | No custom telemetry engine | PostgreSQL+Python E2E, mock cooling adapter, no rebuild |
| 07a | Official OTel OpAMP Supervisor capability spike | Existing Agent Runtime remains sole owner; no ownership transfer | Exact Windows/Linux artifacts, local/remote config composition, fallback, crash/rollback, real OTLP, migration proposal |
| 07b | Go OpAMP server + contract-compatible Go Mock Core | Versioned Core/Agent contracts before dependent code; Core production/shared publication separately approved | Auth/binding, target policy, revision/status semantics, contract conformance, fault injection |
| 07c | Controlled Supervisor runtime adoption | Parity from 07a and contracts/control plane from 07b | Native install/service/offline/recovery, single owner, PR-05/06 parity, Windows/Linux OTLP E2E |
| 08 | Self-health, reporting and diagnostics | No false delivery claims | Agent/Collector/Gateway failure injection |
| 09 | Typed limited operations, security-gated mutation | No shell/SSH, no blind retry | Privilege/auth/target/idempotency/audit tests |
| 09.5 | Mandatory source ownership and context consolidation after behavior stabilizes | No schema, protocol, CLI, monitoring, runtime or ownership behavior change | Import/dependency rules, transaction compatibility, full Windows/Linux regression |
| 10 | Install/upgrade/uninstall packaging, cross-platform release regression | Existing persisted state and dependencies | Full real Windows/Linux gates, release evidence |

## Cross-PR rules

Pure move PRs are reversible and behavior-preserving; never combine DB schema migrations, OTel receiver ownership changes and source moves. Preserve compatibility facades temporarily. PR-05 remains a spike until pinned binary/platform parity is demonstrated; if failing, use existing Agent materialization temporarily and record accepted limitation. PR-07 remote policy starts with only the proposed Core-owned Gateway endpoint; all authority, identity, status, auth, merge, revision, deletion and error contracts must be reviewed before the first dependent slice. PR-07c cannot transfer lifecycle ownership until 07a/07b gates pass. Execute PR-07, PR-08, PR-09, PR-09.5, and PR-10 sequentially. PR-09 must default mutating operations disabled unless all security controls are demonstrably enforceable. PR-09.5 is mandatory after feature behavior stabilizes; it may move ownership without changing behavior or lifecycle authority. PR-10 cannot call unreached gates PASS.

## Plan per PR

`objective` · `current HEAD/verified behavior` · `affected files and context owners` · `invariants` · `stepwise changes` · `edge cases` · `test commands and environment` · `actual evidence` · `rollback` · `open questions`. Evidence report uses PASS/FAIL/BLOCKED/NOT RUN/UNSUPPORTED and exact Git commit.

## Full plan cross-references

See [plan index](README.md) for the implementation checkpoint plans (PR 01–10, including mandatory PR-09.5), with steps, failure cases, acceptance and rollback. Plans are conditional until source evidence is collected.
