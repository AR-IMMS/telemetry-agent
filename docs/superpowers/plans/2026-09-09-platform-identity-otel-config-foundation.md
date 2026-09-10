# Platform Identity and OTel Configuration Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the first working foundation of the AR-IMMS telemetry agent: deterministic platform identity discovery and validated layered OpenTelemetry Collector configuration.

**Architecture:** The Go agent owns platform/host identity, configuration selection, Collector installation/lifecycle orchestration, and diagnostics. The OpenTelemetry Collector owns telemetry receivers, resource detection, processing, batching, retry, and export; Go code must not duplicate those capabilities. Platform-specific behavior stays isolated behind identity/platform files and produces a platform-neutral `PlatformInfo` contract consumed by configuration rendering.

**Tech Stack:** Go standard library, YAML configuration parsing already selected by the repository, OpenTelemetry Collector YAML configuration, Go `testing` package.

**Spec:** `AGENTS.md` and the OTel-centered architecture described in `docs/ARCHITECTURE.md`.

## Global Constraints

- Keep `cmd/agent` composition-only; do not put platform checks, shell commands, or transport details in `main.go`.
- Keep the process split: Go agent supervises `otelcol-contrib`; the Collector performs receiver, processor, batching, retry, and exporter work.
- Preserve the identity hierarchy `physical_host_id -> compute_node_id -> workload_id -> container_id`; container IDs are never the sole historical workload identity.
- Merge configuration layers in this order: `base -> profile -> OS -> vendor -> local machine override`.
- Use explicit deterministic merge semantics for nested objects and lists; do not depend on accidental YAML merge behavior.
- Keep secrets, private keys, tokens, and machine-local overrides out of committed configuration.
- Return contextual errors and make unsupported platforms or invalid merged configuration visible.
- Initial supported platforms are Windows and Linux on `amd64`; do not add speculative platform abstractions.

---

## Current State and File Map

The repository currently contains only stubs for `internal/identity/platform.go` and `internal/identity/platform_windows.go`; `internal/config/` and the requested configuration fragments do not yet exist. The implementation should create focused files with one responsibility each:

- `internal/identity/platform.go` — platform-neutral `PlatformInfo`, `CollectPlatformInfo()`, and deterministic `AsJSON()`.
- `internal/identity/platform_linux.go` — Linux metadata collection and `/etc/os-release` parsing.
- `internal/identity/platform_windows.go` — Windows metadata collection using safe standard-library/runtime information.
- `internal/identity/platform_test.go` — contract tests for platform shape, JSON, and parser behavior.
- `internal/config/types.go` — typed config model for agent settings and Collector rendering inputs.
- `internal/config/renderer.go` — layer loading, deterministic merge, and YAML rendering.
- `internal/config/validator.go` — merged-config validation with contextual errors.
- `internal/config/renderer_test.go` — merge, render, precedence, and failure tests.
- `configs/base/otel.yaml` — platform-neutral Collector pipeline baseline.
- `configs/profiles/laptop.yaml` — laptop profile overrides.
- `configs/os/windows/otel.yaml` — Windows Collector additions/overrides.
- `configs/os/linux/otel.yaml` — Linux Collector additions/overrides.

## Acceptance Criteria

- [ ] `CollectPlatformInfo()` reports normalized OS and architecture, hostname, kernel/release information, and available platform metadata on Windows and Linux.
- [ ] Linux metadata is collected without requiring shell execution and handles missing or malformed `/etc/os-release` values without panicking.
- [ ] Windows metadata is collected without logging secrets or adding a shell-command dependency.
- [ ] `PlatformInfo.AsJSON()` emits stable, valid JSON suitable for diagnostics and tests.
- [ ] Configuration layers load and merge in the documented order, with later scalar values overriding earlier values and nested maps merging explicitly.
- [ ] Lists have a documented deterministic replacement policy rather than accidental concatenation.
- [ ] The renderer selects the OS layer from `PlatformInfo`, renders a Collector YAML document, and rejects invalid merged configuration before Collector startup.
- [ ] Base/profile/OS configuration fragments define an OTel pipeline and do not reimplement Collector receivers, processors, batching, retry, or exporters in Go.
- [ ] Unit tests cover success and failure paths; `gofmt`, `go vet ./...`, `go test ./...`, and `go build ./...` are run and recorded.

## Scope

### In Scope

- Platform identity contract and Windows/Linux metadata adapters.
- Layered Collector configuration model, deterministic merge, rendering, and validation.
- Minimal base, laptop, Windows, and Linux OTel configuration fragments.
- Pure tests and parser tests that do not require a live Collector, gateway, or privileged OS access.

### Out of Scope

- Collector download/install, service registration, restart supervision, or health endpoints.
- Telemetry transport implementation, registration, certificate issuance, or gateway integration.
- Docker/process/hardware source adapters beyond configuration placeholders where the Collector already supplies the capability.
- ARM64 support, vendor-specific profiles, remote configuration, secrets management, and bounded spool implementation.

## Approach

### Task 1: Define platform identity contract and failing tests

**Files:**
- Modify: `internal/identity/platform.go`
- Create: `internal/identity/platform_linux.go`
- Modify: `internal/identity/platform_windows.go`
- Create: `internal/identity/platform_test.go`

**Interfaces:**
- Produces `type PlatformInfo struct` with JSON fields for normalized `os`, `arch`, `hostname`, `kernel`, `release`, and platform metadata.
- Produces `func CollectPlatformInfo() (PlatformInfo, error)`.
- Produces `func (p PlatformInfo) AsJSON() ([]byte, error)`.
- Keeps platform helpers internal so later packages depend only on `PlatformInfo`.

- [ ] Write tests that assert current runtime OS/architecture are populated, hostname is non-empty when the host provides one, and `AsJSON()` is valid and stable across repeated calls.
- [ ] Add table-driven Linux `/etc/os-release` parsing tests for normal, missing, malformed, and duplicate keys.
- [ ] Add tests that unsupported OS values return a clear error through an injectable internal collector helper rather than mutating the real runtime.
- [ ] Run `go test ./internal/identity -run Test -v` and confirm the new tests fail because the contract is not implemented.

### Task 2: Implement minimal platform collection

**Files:**
- Modify: `internal/identity/platform.go`
- Modify: `internal/identity/platform_linux.go`
- Modify: `internal/identity/platform_windows.go`
- Test: `internal/identity/platform_test.go`

**Interfaces:**
- `CollectPlatformInfo()` uses `runtime.GOOS`, `runtime.GOARCH`, `os.Hostname`, and platform helpers.
- Linux helper reads `/etc/os-release` through an injectable file reader and returns normalized distribution metadata.
- Windows helper returns available runtime/kernel metadata without invoking shell commands or embedding credentials.

- [ ] Implement the platform-neutral collector with contextual errors for hostname and platform metadata failures.
- [ ] Implement Linux `/etc/os-release` parsing with quoted-value handling, bounded file reads, and graceful missing-file behavior.
- [ ] Implement Windows metadata with standard-library/runtime APIs and explicit empty values where the host cannot provide a field portably.
- [ ] Implement JSON tags and deterministic field ordering through a fixed struct shape; avoid maps in the public JSON contract unless keys are sorted before encoding.
- [ ] Run `gofmt -w internal/identity/*.go` and `go test ./internal/identity -v`; expected result is PASS.

### Task 3: Define configuration model and validator tests

**Files:**
- Create: `internal/config/types.go`
- Create: `internal/config/validator.go`
- Create: `internal/config/renderer_test.go`

**Interfaces:**
- `type Layer struct { Name string; Path string }` identifies one committed config fragment.
- `type RenderInput struct { Platform identity.PlatformInfo; Layers []Layer }` describes the selected rendering inputs.
- `type Config struct` contains the typed agent/Collector settings needed for validation and rendering.
- `func ValidateConfig(Config) error` rejects missing pipeline, endpoint, or unsupported OS values with field-specific errors.

- [ ] Write tests for required fields, invalid endpoint schemes, unsupported platform selection, and empty pipeline definitions.
- [ ] Write tests that verify list replacement semantics are deterministic and documented.
- [ ] Run `go test ./internal/config -run TestValidate -v` and confirm failure before implementation.
- [ ] Implement YAML tags and only the fields needed by the initial Collector baseline; do not create a generic schema framework.
- [ ] Implement validation with actionable errors and no secret values included in error text.
- [ ] Run `gofmt -w internal/config/*.go` and `go test ./internal/config -run TestValidate -v`; expected result is PASS.

### Task 4: Implement deterministic layered renderer

**Files:**
- Modify: `internal/config/types.go`
- Create: `internal/config/renderer.go`
- Modify: `internal/config/validator.go`
- Test: `internal/config/renderer_test.go`

**Interfaces:**
- `func Render(input RenderInput) ([]byte, error)` loads the selected layers, merges them in order, validates the merged model, and returns Collector YAML.
- Merge order is exactly `base`, `profile`, `os`, then any future vendor/local layers supplied explicitly by the caller.
- Scalar fields from later layers replace earlier values; nested maps merge recursively; lists replace the earlier list as a whole.

- [ ] Add fixture-based tests for base + laptop + Linux and base + laptop + Windows precedence.
- [ ] Add a test proving a later scalar override wins and a test proving a later list replaces rather than silently concatenates.
- [ ] Add a test proving malformed YAML and missing layer files return path-aware errors.
- [ ] Implement layer loading with bounded reads and YAML decoding into the typed model.
- [ ] Implement explicit recursive map/scalar merge behavior using typed structures or a tightly scoped intermediate representation.
- [ ] Validate the fully merged model before encoding it; never start or invoke the Collector from the renderer.
- [ ] Run `go test ./internal/config -run TestRender -v`; expected result is PASS.

### Task 5: Add initial OTel configuration fragments

**Files:**
- Create: `configs/base/otel.yaml`
- Create: `configs/profiles/laptop.yaml`
- Create: `configs/os/windows/otel.yaml`
- Create: `configs/os/linux/otel.yaml`
- Modify: `internal/config/renderer_test.go`

**Interfaces:**
- Fragments are input data only; Go reads and renders them.
- The rendered document contains an OTel Collector service pipeline with receiver, processor, and exporter references.

- [ ] Define the base OTLP pipeline and gateway endpoint placeholders without committing real endpoints or credentials.
- [ ] Define laptop profile resource attributes and bounded collection defaults.
- [ ] Define Windows and Linux OS-specific receiver/resource settings only where the Collector supports them.
- [ ] Add tests that render both OS variants and assert required `receivers`, `processors`, `exporters`, and `service.pipelines` sections exist.
- [ ] Ensure all local paths and credentials remain override-only and are not committed.

### Task 6: Verify and document the foundation

**Files:**
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/README.md` if needed
- Modify: `docs/superpowers/plans/2026-09-09-platform-identity-otel-config-foundation.md`

- [ ] Document the Go-agent/OTel responsibility split and the platform identity/configuration flow in `docs/ARCHITECTURE.md`.
- [ ] Run `gofmt -w internal/identity/*.go internal/config/*.go`.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `go build ./...`.
- [ ] Record command results, known platform limitations, and follow-up work in this plan's Completion Summary.

## Risks and Decisions

- Risk: OS-specific metadata APIs differ and may not be available on the host running tests.  
  Mitigation: isolate helpers behind small interfaces, use parser tests and current-platform tests, and avoid requiring privileged access.

- Risk: Generic YAML merging can silently concatenate or overwrite nested values incorrectly.  
  Mitigation: define merge semantics explicitly, test scalar/map/list behavior, and validate only the fully merged result.

- Risk: Collector configuration may refer to components unavailable in the pinned Collector build.  
  Mitigation: keep the initial fragments minimal and add a Collector-config validation check before enabling broader receivers.

- Risk: Platform identity fields could become an accidental public persistence contract.  
  Mitigation: keep `PlatformInfo` focused on normalized discovery data and defer registration/schema changes to a reviewed ADR.

- Decision needed: confirm the YAML library already used or approved by the Go module before implementation; do not add a new dependency if an existing project utility satisfies decoding/encoding needs.

## Verification

- [ ] Unit tests: `go test ./internal/identity ./internal/config -v`
- [ ] Integration / contract tests: render both Linux and Windows fixture configurations and validate required OTel sections; live gateway tests are out of scope.
- [ ] Lint / typecheck / build: `gofmt -w ...`, `go vet ./...`, `go build ./...`
- [ ] Manual verification: inspect rendered YAML for deterministic layer precedence and absence of secrets or real endpoints.

## Implementation Notes

- The implementation must not invoke shell commands for platform detection or metadata collection.
- `PlatformInfo` should remain platform-neutral at the contract level; OS-specific fields should be represented as optional metadata, not duplicated platform-specific public types.
- The renderer must not own Collector process startup; it returns validated configuration to the future bootstrap/supervisor packages.
- If the initial requirements expand to registration, transport, spool, or service lifecycle, split those into separate plans because they are independent subsystems.

## Completion Summary

- Changed: Plan only; implementation not started.
- Verification: Plan reviewed against `AGENTS.md`, `.harness/AGENTS.md`, and `.harness/templates/plan.md`.
- Known limitations: `docs/PRODUCT.md` and `docs/ARCHITECTURE.md` are currently empty; this plan captures the architecture needed for the first implementation slice.
- Follow-up: Obtain approval to execute this plan, then choose subagent-driven or inline execution.
