# AR-IMMS Telemetry Agent

## 1. Project Identity

This repository contains a lightweight, cross-platform telemetry agent for the
AR-assisted Infrastructure Monitoring and Maintenance System (AR-IMMS).

The agent runs on Windows and Linux laptops or server-like nodes. It detects
the host, installs and supervises a pinned OpenTelemetry Collector, collects
host/workload/hardware telemetry, normalizes identity and metadata, and sends
telemetry to a central gateway or ingestion boundary.

This is a focused proof of concept for at most three laptops/nodes. It is not a
reimplementation of the Datadog Agent and must not grow into one.

## 2. Runtime Topology

The Go agent and the OpenTelemetry Collector are separate processes:

```text
- agent
  - bootstrap / installation
  - configuration rendering
  - identity and discovery
  - Collector supervision
  - diagnostics and lifecycle
  - bounded local spool

otelcol-contrib
  - receivers
  - processors
  - resource detection
  - batching
  - retry
  - exporters
```

The product may be distributed as one installer or package containing both
binaries, but the processes remain independently restartable and observable.

The Go agent must not reimplement OTel Collector receivers, processors,
exporters, batching, retry, or resource detection when Collector configuration
can provide the capability.

## 3. Responsibilities

### Go Agent Owns

- platform and architecture detection;
- physical host, compute node, vendor, and workload identity;
- dependency installation and configuration rendering;
- Windows Service and Linux systemd lifecycle;
- Collector start, health checks, restart policy, upgrade, and uninstall;
- OS, vendor, and workload discovery adapters;
- bounded local memory/disk spool when the gateway is unavailable;
- node registration, certificate handling, and diagnostics.

### OpenTelemetry Collector Owns

- telemetry receiving and collection where a receiver exists;
- resource detection available through Collector components;
- processing, batching, retry, and export;
- OTLP and exporter protocol handling.

### Out of Scope

The agent does not own alerting, incident creation, dashboard behavior,
topology persistence, AI/anomaly detection, or control-plane business logic.

## 4. Technology Direction

- Language: Go.
- Telemetry engine: pinned `otelcol-contrib` release.
- Initial platforms: Windows and Linux.
- Initial architecture: `amd64`; add `arm64` only with a tested need.
- Host sources: OTel host metrics and/or Windows exporter through a supported
  Collector receiver.
- Hardware sources: LibreHardwareMonitor on Windows and platform-specific
  adapters only where generic sources are insufficient.
- Workload sources: Docker API, Windows services/processes, Linux systemd and
  processes, and controlled simulation producers, and many more.
- Default transport: OTLP to an OTel gateway.
- Custom gRPC/mTLS: allowed only when required by the existing ingestion
  boundary; keep it behind the transport interface.
- Packaging: Go agent binary, pinned Collector binary, configuration, and
  Windows/Linux service assets.

## 5. Architectural Boundaries

Keep these responsibilities in separate packages:

```text
bootstrap  -> detect, install, configure
supervisor -> start, observe, restart the Collector
config     -> layered rendering and validation
identity   -> host, node, vendor, and workload identity
platform   -> Windows and Linux implementations
sources    -> host, exporter, Docker, process, and hardware readers
normalize  -> canonical telemetry and domain models
spool      -> bounded outage buffering
transport  -> authenticated delivery to the gateway
health     -> readiness and diagnostic checks
```

Rules:

- Keep `main.go` limited to composition and process startup.
- Do not put vendor checks, OS shell commands, or transport details in
  `main.go`.
- Use small interfaces at real boundaries, not speculative abstractions.
- Keep platform-specific code isolated behind explicit implementations.
- Transport adapters deliver telemetry; they do not own domain business logic.
- Public APIs, telemetry schemas, event contracts, and persistence contracts
  require explicit review and usually an ADR.

## 6. Identity Model

The identity hierarchy is:

```text
physical_host_id -> compute_node_id -> workload_id -> container_id
```

Definitions:

- `physical_host_id`: stable identity of the physical laptop or server.
- `compute_node_id`: stable identity of the monitored agent installation/node.
- `workload_id`: stable logical identity of a service, process, or workload
  where one can be established.
- `container_id`: runtime-specific, ephemeral Docker container identifier.

Container IDs must not be used as the sole identity for historical workload
data. Preserve the distinction between current inventory and historical
telemetry identity.

## 7. Configuration

Configuration layers merge in this order:

```text
base -> profile -> OS -> vendor -> local machine override
```

Later layers override earlier scalar values. Nested objects and lists must use
explicit, deterministic merge semantics; do not rely on accidental YAML merge
behavior.

Rules:

- Do not duplicate complete configuration files for every laptop model.
- Keep machine-local overrides and secrets out of version control.
- Validate the fully merged configuration before starting the Collector.
- Record the source layer of important identity, endpoint, and security values.
- Generated configuration must be reproducible from committed inputs.

## 8. Source Selection Rule

Before writing custom Go collection code, check capabilities in this order:

1. OTel native receiver or processor.
2. Existing local exporter consumed through OTel.
3. Small platform/workload adapter with a stable interface.
4. Custom Go collection only when the previous options are insufficient.

Document the reason when choosing a lower option. Do not create a custom
collector merely to avoid configuring an existing OTel component.

## 9. Repository Structure

```text
cmd/          executable entrypoints (`agent`, `agentctl`)
internal/     private implementation packages
  bootstrap/  installation and first-run setup
  supervisor/ Collector process/service supervision
  config/     layered config rendering and validation
  identity/   host, node, vendor, and workload identity
  platform/   Windows and Linux implementations
  sources/    host, exporter, Docker, process, and hardware readers
  normalize/  canonical telemetry/domain models
  spool/      bounded memory/disk buffering
  transport/  OTLP or authenticated gRPC delivery
  health/     diagnostics and readiness checks
configs/      base + profile + OS + vendor fragments
packaging/    Windows and Linux install/service assets
schemas/      configuration and telemetry contracts
scripts/      development and packaging helpers
tests/        integration and end-to-end tests
docs/         architecture, ADRs, runbooks, and product notes
.harness/     coding workflow, templates, and review rules
```

## 10. Coding Rules

- Keep `main.go` and entrypoints thin; compose dependencies, do not contain
  business logic there.
- Prefer small, focused packages with explicit interfaces at real boundaries.
- Keep platform-specific code behind platform-specific implementations.
- Return errors with useful context; do not silently swallow or downgrade
  failures.
- Make installation, configuration, start, stop, restart, and uninstall
  operations idempotent.
- Bound retries, subprocesses, goroutines, memory, and disk usage explicitly.
- Use structured logs; never log secrets, tokens, private keys, or full
  authorization headers.
- Prefer standard library and existing project utilities before adding a
  dependency or abstraction.
- Search for an existing helper before creating a new wrapper or adapter.
- Do not edit generated files directly; modify their source and regenerate.
- Keep OS/vendor branches out of shared normalization and transport logic.
- Add a regression test for every fixed bug where practical.
- Do not add speculative abstractions, plugin systems, or generic frameworks.

## 11. Quality Bar

Every change must be:

- deterministic and idempotent when install/configure operations are rerun;
- explicit about permissions, platform assumptions, and failure behavior;
- bounded in memory, retries, subprocesses, and disk usage;
- observable through structured logs and health/diagnostic output;
- testable without a physical data center;
- platform-neutral at the contract level, or clearly isolated to one platform;
- documented when it changes a contract, deployment behavior, or security model.

## 12. Testing Expectations

Prioritize tests in this order:

1. Pure tests for identity, config merge, normalization, retry, and spool.
2. Fake platform and service-manager tests.
3. Source adapter tests using recorded or simulated input.
4. Collector configuration validation tests.
5. Windows/Linux integration tests.
6. Three-node end-to-end tests through the gateway.

Failure tests are first-class. Cover, where relevant:

- gateway unavailable;
- invalid or expired certificate;
- invalid or expired configuration;
- missing hardware sensor;
- Docker permission or context failure;
- Collector crash and restart;
- host reboot;
- duplicate delivery;
- full local spool.

For Go changes, run the narrowest applicable checks first:

```text
gofmt -w <changed Go files>
go vet ./...
go test ./...
go build ./...
```

Run platform-specific integration tests for the affected platform. Do not
claim a cross-platform change is complete based only on a build on one OS.

## 13. Security Rules

- Never commit private keys, client certificates, tokens, or real endpoints.
- Never log credentials or full authorization headers.
- Bind local receivers to localhost unless a network listener is required.
- Use least privilege and document administrator/root requirements.
- Validate downloaded artifacts before installation.
- Treat registration and certificate issuance as explicit operations.
- Make authorization failures visible; never silently downgrade to insecure
  transport.

## 14. Development Workflow

For every feature or meaningful change:

1. State the problem, scope, and observable acceptance criteria.
2. Load the relevant project context and contracts.
3. Check whether OTel already provides the capability.
4. Decide whether an ADR is required.
5. Define the smallest boundary and platform impact.
6. Add or update tests before implementation where practical.
7. Update affected docs, contracts, or runbooks.
8. Run verification and record meaningful limitations.
9. Review the change against both the plan and the architecture.

Use `docs/adr/` for architectural decisions and `docs/runbooks/` for
installation, recovery, certificate, and troubleshooting procedures.

## 15. Scope Guardrails

Do not add these without an explicit ADR and demonstrated PoC need:

- dynamic remote code execution or arbitrary plugin loading;
- a custom query language;
- a distributed control plane inside the agent;
- a full package manager or auto-update service;
- AI or anomaly detection inside the node agent;
- a replacement for the OpenTelemetry Collector;
- Datadog-style multi-process orchestration or framework abstraction.

The immediate delivery path is:

```text
Phase 1: bootstrap, installation, service lifecycle, and health
Phase 2: collection, normalization, queue, and transport
Phase 3: secure registration, vendor expansion, simulation, and validation
```

When a proposal does not clearly serve one of these phases, defer it or create
an ADR before implementation.

## Code documentation

- Every package must have a package-level Go doc comment in `doc.go`.
- The package comment must explain:
  1. the package responsibility;
  2. its main input/output or result;
  3. important boundaries: what it intentionally does not own.
- Exported types, functions, methods, and constants must have Go doc comments.
- Comments must explain intent, invariants, security assumptions, or non-obvious
  decisions. Do not restate obvious syntax.
- Keep comments accurate when behavior changes. Outdated documentation is a bug.
- Prefer short English comments that start with the exported identifier name.

Example:
```go
// SelectArtifact returns the committed Collector artifact supported by platform.
func SelectArtifact(platform identity.PlatformInfo) (Artifact, error)
```
