# Proposed Source Structure and Migration Map

**Structure note (mandatory):** This directory tree is a reference baseline, not a mandatory layout; adapt or extend it (e.g., `infrastructure/`, additional adapters, or different subpackages) when justified by domain complexity, while preserving bounded-context ownership, clear Domain/Application/Infrastructure responsibilities, and dependency rules.

**Status:** PROPOSED — Context-oriented architecture with lightweight Clean Architecture principles.

## 1. Design Principles

- Organize code by **Bounded Context first**, then by responsibilities.
- **Domain:** Business entities, value objects, invariants, and domain rules.
- **Application:** Use cases, orchestration, transactions, and coordination through ports.
- **Adapters:** SQLite, HTTP, Docker, OpAMP, OS services, Collector integration.
- Create subpackages only when they improve ownership or navigation. Avoid unnecessary abstraction.
- Each context owns its domain models and persistence contracts.
- Prefer explicit dependencies, small interfaces, and composition over frameworks.


## 2. Proposed Structure, Contract Ownership & Organization

**Mandatory:** Every cross-context and cross-system interaction must have an explicit, discoverable, and testable contract.

### Proposed Contract Structure

```text
internal/
├── identity/
│   ├── domain/
│   ├── application/
│   ├── contracts/             # Public identity capabilities and DTOs
│   └── adapters/
│
├── configuration/
│   ├── domain/
│   ├── application/
│   ├── contracts/             # Desired state, revisions, apply requests
│   └── adapters/
│
├── inventory/
│   ├── domain/
│   ├── application/
│   ├── contracts/             # Resource observations and inventory queries
│   └── adapters/
│
├── monitoring/
│   ├── domain/
│   ├── application/
│   ├── contracts/             # Monitoring intents and capability resolution
│   └── adapters/
│
├── runtime/
│   ├── domain/
│   ├── application/
│   ├── contracts/             # Activation, readiness, recovery results
│   └── adapters/
│
├── health/
│   └── contracts/             # Health evidence and status projections
│
└── operations/
    └── contracts/             # Authorized operation requests/results

contracts/                     # Cross-system protocol specifications
├── README.md
├── core/                      # Agent ↔ Core API, OpAMP policy/status
├── telemetry/                 # OTLP and monitoring interoperability
└── operations/                # Remote operations protocol
```

**Structure rule:** These paths are illustrative. A small context may expose its contract directly from its root package; do not create empty `contracts/` directories.

### Internal Contract Rules

- The providing context owns its public contract and guarantees its behavior.
- Consumers depend only on exported contracts, not internal implementation or persistence.
- Prefer consumer-defined Go interfaces at the point of use, with provider-owned public request/response types where appropriate.
- Define input/output types, validation, errors, lifecycle, idempotency, concurrency, and failure semantics.
- Cross-context events must have explicit payloads, ownership, and delivery guarantees.
- Avoid shared mutable domain objects and circular dependencies.
- Every contract change requires compatibility analysis and consumer contract tests.

### External Contract Rules

- Store Agent-owned protocol specifications under repository-level `contracts/`.
- **`docs/shared/` remains the source of truth for approved cross-repository AR-IMMS contracts.** Local specifications are drafts until approved and synchronized.
- Define protocol/schema versions, authentication, authorization, request/response formats, error mapping, timeouts, retries, idempotency, and compatibility.
- Reference standard protocols such as OTLP and OpAMP rather than duplicating their complete specifications.
- Validate integrations with schema/contract tests and real system E2E tests.
- Never silently change an external contract to make a local implementation pass.

### Contract Change Gate

Before implementing or modifying a boundary, identify:

1. **Owner and consumers**
2. **Contract location and version**
3. **Inputs, outputs, errors, and side effects**
4. **Invariants and failure/retry semantics**
5. **Compatibility and migration impact**
6. **Required unit, contract, integration, and E2E tests**

Every PR verification report must list affected contracts, breaking changes, and test evidence.

**Principle:** Contracts are architectural boundaries, not just shared structs or documentation files. Keep them small, explicit, independently testable, and owned by the appropriate context.

## 3. Dependency Rules

```text
CLI → Application → Domain
          ↓
       Ports
          ↑
       Adapters

internal/app → Wires all dependencies
```

- Domain must not depend on application or adapters.
- Application depends on domain and small ports, not concrete infrastructure.
- Adapters implement ports and may depend on domain types.
- Cross-context access uses public contracts, never another context's internal persistence.
- `platform/` must not contain business entities.
- `internal/app` performs dependency injection and lifecycle wiring, not business orchestration.
- Avoid generic repositories, service locators, reflection-based DI, or catch-all `common/utils` packages.

## 4. Existing → Target Migration

| Existing | Target |
|---|---|
| `cmd/agentctl` orchestration | `internal/app` + owning application use cases |
| `identity`, `enrollment`, `coreapi`, `credentialstore` | `identity/` |
| `coreconfig` | `identity/` or `configuration/`, depending on authority ownership |
| `discovery` | `inventory/` |
| `integration` definitions, matching, Redis | `monitoring/` |
| Desired Integration instances/Templates | `configuration/` |
| `configuration`, `reconciliation` | `configuration/` |
| `agentlifecycle`, `collectorruntime`, `bootstrap`, `supervisor` | `runtime/` |
| `dependency`, `agentinstallation` | `runtime/adapters/` |
| `agenthealth` | `health/` |
| `agentstate` | Context-owned SQLite adapters + `platform/sqlite` |

The current `internal/supervisor` is the Agent's Collector process supervisor;
it is not OTel OpAMP Supervisor. Treat any future OpAMP server adapter and
Supervisor provisioning as explicit boundaries, not a reason to mechanically
rename or move existing packages. See the [PR-07 migration contract](opamp-integration.md).

Preserve compatibility facades during migration. Do not mechanically move packages or change SQLite schema while reorganizing ownership.

## 5. Refactoring & Compatibility Rules

1. Preserve existing enrollment, identity, credentials, and SQLite state.
2. Preserve public CLI behavior, flags, output, and exit codes.
3. Preserve Collector generation, validation, readiness, and Last-Known-Good recovery.
4. Preserve managed dependency ownership and uninstall safeguards.
5. Preserve current monitoring and Redis E2E behavior until OTel-native replacements pass parity tests.
6. Separate structural refactoring from functional changes.
7. Keep production Go files within the repository's coding policy limits.
8. Document exported contracts, domain invariants, concurrency, transaction boundaries, and security-sensitive behavior.
9. Validate Windows and Linux builds and relevant tests after each migration slice.

**Architecture objective:** Make code easy to find, understand, extend, and maintain without introducing unnecessary layers or frameworks.
## 6. PR-01 source baseline and guard scope

The current implementation remains in its existing package structure; this baseline does not require a move into the proposed tree. `cmd/agentctl` currently composes and routes workflows. `agentstate` is a central SQLite adapter that imports several contexts, while `agentlifecycle` and `reconciliation` coordinate runtime and desired-state packages. These are migration seams, not a reason to introduce duplicate adapters in PR-01.

PR-01 adds a conservative CI guard for two dependency directions supported by the present source: internal packages cannot import the `cmd/agentctl` composition root, and the existing `configuration`, `discovery`, `enrollment`, and `integration` packages cannot import persistence, service lifecycle, Collector execution, or external transport adapters. Context-to-context contract dependencies remain allowed. Current orchestration and persistence coupling stays explicit and transitional; it is not silently treated as the target design.

The same check caps non-test, non-generated production Go files at 450 lines. This is an automated ceiling only; it does not waive the repository's normal file-size justification guidance. The check does not require a directory layout, split files mechanically, or classify the proposed package tree as mandatory. The shared coding policy must be available before confirming policy alignment.

The initial transition allowlist records the existing `identity` package's direct Windows registry access. The import guard does not allow other domain-to-platform edges. This exception is evidence for a future platform-adapter migration, not a general permission to add OS imports to domain logic.

The verified baseline also found `internal/enrollment/service.go` is 454 lines, already four lines above the proposed 450-line ceiling. PR-01 records a file-specific ceiling of 454 so it cannot grow; reduce it only during a meaningful context migration, not by splitting code mechanically.


The CI guard also checks the repository's existing `doc.go` rule for every Go source directory under `cmd/` and `internal/`; it does not create package directories or impose a proposed context layout.
