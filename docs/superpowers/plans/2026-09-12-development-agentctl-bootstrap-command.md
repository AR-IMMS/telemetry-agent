# Implementation Plan: Development `agentctl bootstrap` Command

## Context

- Issue / request: `docs/requirements/development-agentctl-bootstrap-command.md`
- Owner: telemetry-agent maintainers
- Status: Draft
- Created: 2026-09-12

## Objective

Provide a development-only `agentctl bootstrap` command that runs the existing
Collector bootstrap flow from local configuration fragments. The command must
select platform configuration deterministically, validate and activate the
rendered Collector configuration through `bootstrap.Run`, and report the
installation result without starting the Collector or contacting a gateway.

## Acceptance Criteria

- [ ] `bootstrap` accepts required `--config-root`, `--install-dir`, and
      `--config-path` flags; it rejects missing and unknown flags with concise
      usage errors and a non-zero exit status.
- [ ] The command calls `identity.CollectPlatformInfo()` and supplies base,
      laptop profile, and detected Linux or Windows layer paths to
      `config.RenderInput` in that order.
- [ ] `bootstrap.Run` receives the requested installation/configuration paths,
      the selected `config.RenderInput`, a validation-only
      `OTEL_GATEWAY_ENDPOINT` value, and the default or explicit validation
      timeout.
- [ ] Successful execution reports the pinned Collector version, binary path,
      final configuration path, and whether the installation was reused.
- [ ] Bootstrap failures preserve their context, return a non-zero status, and
      never emit the success summary.
- [ ] Tests do not download a Collector artifact or launch a real Collector.
- [ ] `go test ./cmd/agentctl -v`, `go test ./...`, `go vet ./...`, and
      `go build ./...` pass.

## Scope

### In Scope

- A thin `cmd/agentctl` administrative CLI entrypoint with a single
  development-only `bootstrap` subcommand.
- Flag parsing, platform/layer selection, composition of existing bootstrap
  dependencies, concise output, and offline unit tests.
- Linux and Windows configuration-layer selection covered through injected
  platform information in tests.

### Out of Scope

- Starting or supervising the Collector.
- Windows Service or systemd registration.
- Gateway communication, node registration, certificates, or secrets.
- New configuration merge/rendering, artifact installation, or Collector
  validation behavior inside `internal/*`.
- Additional `agentctl` lifecycle or diagnostic subcommands.

## Current State

`internal/bootstrap.Run` already renders `config.RenderInput`, installs or
reuses the pinned Collector, validates a staged configuration with the
Collector binary, and atomically activates the final path. It accepts injected
`Downloader` and `CommandRunner` dependencies, while `identity.CollectPlatformInfo`
provides the OS and architecture required for artifact selection.

The committed configuration fragments already exist at `configs/base/otel.yaml`,
`configs/profiles/laptop.yaml`, `configs/os/linux/otel.yaml`, and
`configs/os/windows/otel.yaml`. No `cmd/agentctl` package or executable exists
yet, so the command must introduce the composition boundary without duplicating
the logic that is already owned by `bootstrap`, `config`, or `identity`.

## Approach

1. Add the first Linux-capable tracer bullet: a thin `agentctl bootstrap`
   command that parses its contract, collects platform identity, derives the
   three ordered configuration layer paths, supplies the validation-only
   endpoint environment, invokes existing bootstrap orchestration, and prints
   a concise result. Keep the command's platform collection and bootstrap
   invocation behind narrow package-local seams so its unit tests use fakes.
2. Expand the same command to complete its external contract: support the
   Windows layer path through the detected platform, cover every usage-error
   path and optional-flag behavior, and ensure bootstrap failures cannot be
   mistaken for success. This task depends on the first command slice, because
   it hardens its stable composition boundary rather than creating another CLI
   implementation.
3. Run the requirement-specified package and repository checks. Record that
   the tests prove command construction and result handling only; they do not
   execute a real Collector download or subprocess.

## Affected Areas

- `cmd/agentctl/main.go` — compose the development-only CLI, existing
  bootstrap dependencies, platform identity, and output.
- `cmd/agentctl/main_test.go` — offline tests for arguments, layer selection,
  environment/timeout options, failure behavior, and success output.
- `docs/superpowers/plans/2026-09-12-development-agentctl-bootstrap-command.md`
  — approved delivery plan and task dependency record.
- Contracts affected: CLI behavior and configuration-layer selection only; no
  API, telemetry schema, or persistent-state contract changes.

## Risks and Decisions

- Risk: tests accidentally use the production downloader or process runner.  
  Mitigation: route bootstrap invocation through a package-local function or
  interface seam; tests replace it with a recording fake and assert its input.

- Risk: flag parsing becomes coupled to business behavior and makes required
  error cases hard to test.  
  Mitigation: separate argument parsing from the small command execution
  function, with explicitly supplied input/output streams and dependencies.

- Risk: validation endpoint data is written into generated configuration or
  treated as a production gateway setting.  
  Mitigation: pass only `OTEL_GATEWAY_ENDPOINT=<value>` in
  `bootstrap.Options.ValidationEnvironment`; do not add it to render values,
  configuration fragments, or logs.

- Decision needed: None. The requirement fixes the configuration profile,
  supported operating systems, defaults, and no-lifecycle boundary.

## Verification

- [ ] Unit tests: `go test ./cmd/agentctl -v`
- [ ] Integration / contract tests: fake bootstrap invocation verifies selected
      layers, validation environment, timeout, result output, and failure
      output without downloading or executing a Collector.
- [ ] Lint / typecheck / build: `gofmt -w cmd/agentctl/main.go cmd/agentctl/main_test.go`,
      `go test ./...`, `go vet ./...`, `go build ./...`
- [ ] Manual verification: run `go run ./cmd/agentctl bootstrap` with temporary
      local paths only after the Collector artifact is intentionally available;
      confirm it validates/activates configuration but does not start a process
      or register a service.

## Implementation Notes

### Task 1: Implement Linux `agentctl bootstrap` tracer bullet

**Blocked by:** None (can start immediately)

Deliver a runnable `agentctl bootstrap` flow for the detected Linux platform.
It must enforce the required path flags, apply the specified defaults, resolve
the base/profile/Linux fragments in order, call existing bootstrap orchestration
with real production dependencies, and produce the required success report.
Tests use injected fake platform/bootstrap operations to prove the complete
composition path without external downloads or subprocesses.

- [ ] Add the package-level documentation and thin command entrypoint.
- [ ] Implement subcommand and flag parsing with concise usage errors.
- [ ] Construct ordered `config.Layer` values and `bootstrap.Options` from
      collected platform identity and flags.
- [ ] Invoke `bootstrap.Run` through the narrow test seam using
      `bootstrap.HTTPDownloader` and `bootstrap.OSCommandRunner` in production.
- [ ] Add fake-driven happy-path, ordered-layer, default endpoint, explicit
      endpoint, options-passing, bootstrap-failure, and success-output tests.
- [ ] Run `go test ./cmd/agentctl -v` and commit the independently working
      Linux slice.

### Task 2: Complete cross-platform and CLI failure contract

**Blocked by:** Task 1: Implement Linux `agentctl bootstrap` tracer bullet

Extend the established command without changing its ownership boundaries.
It must select the Windows OS fragment for Windows identity and fulfill the
remainder of the command's observable CLI contract, especially all malformed
invocation behavior and the guarantee that success output appears only after a
successful bootstrap result.

- [ ] Add Windows platform-selection coverage for
      `configs/os/windows/otel.yaml` while retaining Linux coverage.
- [ ] Add tests for unsupported subcommands, unknown flags, and each missing
      required flag; assert a non-zero result and concise usage error.
- [ ] Assert the two-minute default validation timeout and explicit timeout
      override passed into bootstrap options.
- [ ] Assert bootstrap failures preserve their error context and suppress all
      success-only result fields.
- [ ] Run `go test ./cmd/agentctl -v`, then the full repository verification
      commands, and commit the completed command contract.

## Completion Summary

- Changed: Plan only; implementation has not started.
- Verification: Requirement, current bootstrap/config/identity contracts, and
  existing configuration fragment locations were reviewed.
- Known limitations: The planned tests deliberately do not exercise real
  artifact download or Collector process execution.
- Follow-up: Publish the two approved tickets to the configured tracker (or
  configure the local Markdown tracker), then implement Task 1 before Task 2.
