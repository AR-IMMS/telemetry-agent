# Implementation Plan: Bootstrap Pinned OpenTelemetry Collector

## Context

- Issue / request: Implement the next Phase 1 step after platform identity and rendered configuration.
- Owner: Telemetry agent maintainers
- Status: Draft
- Created: 2026-09-10

## Objective

Add an idempotent bootstrap foundation that selects a pinned `otelcol-contrib`
artifact for Windows/Linux amd64, verifies and installs it safely, renders the
Collector configuration, and runs the Collector's native configuration
validation. This phase must not register or start a Windows Service/systemd
unit.
Precodition must watch: Implementation may not begin until the manifest contains one verified artifact for each supported target: windows/amd64 and linux/amd64.

## Acceptance Criteria

- [ ] Only committed, pinned artifact metadata can select a Collector download.
- [ ] Unsupported OS/architecture combinations fail before download.
- [ ] Downloads are bounded, checksum-verified, and staged before installation.
- [ ] Archive extraction rejects path traversal and unexpected executable layout.
- [ ] Installation stages a verified artifact in a temporary directory, verifies the expected binary checksum after extraction, then atomically renames the completed version directory into place. A matching install is reused only after its installed binary checksum and installation metadata match the pinned manifest.
- [ ] Configuration is rendered and written atomically before validation.
- [ ] `otelcol-contrib validate --config <path>` runs without shell interpolation,
      with timeout and bounded output handling.
- [ ] Validation failure prevents bootstrap success and returns contextual errors.
- [ ] No service registration, Collector start, gateway call, registration, or
      certificate issuance occurs in this phase.
- [ ] Failure paths have unit tests and the package passes Go verification.
- [ ] Architecture and operational documentation describe the new boundary.

## Scope

### In Scope

- `internal/bootstrap` artifact manifest, platform selection, download,
  checksum verification, safe extraction, atomic installation, and validation.
- Bootstrap result/options contracts and dependency seams for deterministic tests.
- Pinned artifact metadata for Windows/Linux amd64.
- Local rendered-config installation and `otelcol-contrib validate` execution.
- Unit tests using fake downloaders, command runners, temporary directories, and
  fixture archives.
- Documentation and an ADR for artifact trust and installation layout.

### Out of Scope

- Windows Service or systemd registration and lifecycle management.
- Collector start/stop/restart supervision or health endpoint checks.
- Collector upgrade/rollback policy beyond safe replacement of the pinned binary.
- Gateway connectivity, registration, certificates, mTLS, telemetry adapters,
  Docker, hardware, vendor profiles, and disk spool.
- Dynamic remote configuration, arbitrary plugin loading, or unverified binaries.

## Current State

`internal/identity` provides platform identity and `internal/config` renders and
structurally validates layered Collector YAML. Configuration fragments exist for
base, laptop, Windows, and Linux profiles. There is currently no `bootstrap`
package, artifact manifest, Collector binary, installation directory contract,
or subprocess wrapper. The repository supports Windows/Linux amd64 only for this
phase. The pinned distribution's component semantics are intentionally deferred
to `otelcol-contrib validate`.

## Approach

1. Record the artifact trust, installation layout, permissions, and replacement
   strategy in an ADR before implementation.
2. Define `internal/bootstrap` contracts for artifact metadata, platform
   selection, bootstrap options/results, downloads, command execution, and
   validation errors.
3. Add a committed manifest for the exact Collector version, OS, architecture,
   URL, SHA-256, archive format, and expected executable name.
4. Implement bounded downloads into a temporary staging directory, checksum
   verification, safe archive extraction, and executable discovery.
5. Implement idempotent atomic installation. Preserve an existing verified
   installation until the replacement has passed all staging checks.
6. Integrate the existing identity/config packages to render the selected
   layers and write the rendered YAML atomically to the installation config path.
7. Invoke the installed Collector with a context timeout and argument array:
   `validate --config <path>`. Capture bounded output and expose exit failures.
8. Add orchestration tests covering success, rerun, unsupported platform,
   checksum mismatch, oversized download, traversal, validation failure, and
   timeout.
9. Run targeted, cross-platform, and repository-wide verification; update the
   plan completion summary with evidence and limitations.

## Affected Areas

- `internal/bootstrap/` — new bootstrap orchestration and safe installation code.
- `configs/` or `schemas/` — committed artifact manifest source, if represented
  as YAML/JSON rather than Go data.
- `internal/identity/` — consumed for OS/architecture selection; no contract
  changes expected.
- `internal/config/` — consumed for rendering; no Collector semantic validation
  duplication.
- `docs/adr/` — artifact trust and installation layout decision.
- `docs/ARCHITECTURE.md` and `docs/README.md` — Phase 1 bootstrap boundary and
  local verification instructions.
- Contracts affected: installation/deployment contract and bootstrap API;
  Collector configuration output remains the existing contract.

## Risks and Decisions

- Risk: A downloaded archive may contain traversal paths or unexpected files.  
  Mitigation: inspect archive entries before extraction, reject absolute paths,
  `..` components, symlinks, and files outside the expected layout.

- Risk: Checksum-only verification may not satisfy future supply-chain policy.  
  Mitigation: require SHA-256 now and keep verification behind a small seam so
  signature/provenance verification can be added without changing orchestration.

- Risk: Replacing a binary in use can fail on Windows.  
  Mitigation: bootstrap does not stop services in this phase; install to a
  versioned/staged location and make replacement behavior explicit for the next
  lifecycle phase.

- Risk: Collector validation may reject environment-variable endpoint syntax or
  unavailable components.  
  Mitigation: run the pinned binary's validation as the source of truth and
  report its bounded stderr/stdout; do not weaken Go structural validation.

- Decision needed: confirm the initial artifact source and exact pinned
  `otelcol-contrib` release/checksums before implementation. No real production
  endpoint or credential belongs in the repository.

- ADR: Required because this changes artifact trust, installation layout, and
  deployment behavior.

## Verification

- [ ] Unit tests: `go test ./internal/bootstrap ./internal/config ./internal/identity -v`
- [ ] Integration/contract tests: fake Collector executable validates argument
      construction, timeout, bounded output, and non-zero exit handling.
- [ ] Security tests: checksum mismatch, oversized artifact, archive traversal,
      symlink rejection, and unexpected executable layout.
- [ ] Cross-platform checks: `GOOS=linux GOARCH=amd64 go test -c ./...` and
      `GOOS=windows GOARCH=amd64 go test -c ./...` where packages permit.
- [ ] Lint/build: `gofmt`, `go test ./...`, `go vet ./...`, `go build ./...`.
- [ ] Manual verification: inspect the staged/install tree and confirm no
      service registration or Collector start occurs.

## Implementation Notes

- Use `exec.CommandContext`; never invoke a shell or concatenate command lines.
- Bound network reads, archive size, subprocess runtime, and captured output.
- Do not log rendered configuration, credentials, authorization headers, or
  full Collector command output when it may contain secrets.
- Use atomic temporary-file writes for configuration and manifest metadata.
- Keep filesystem/process seams small and concrete; avoid a generic package
  manager abstraction.
- The next plan owns service registration, start/stop/restart, health checks,
  and `agentctl status/doctor`.
  - Overall implement flow (easy to track)
     - 1. ADR: trust root + paths + versioned install layout
    - 2. Commit exact Windows/Linux artifact manifest
    - 3. Artifact selection tests
4. Bounded download + checksum tests
5. Safe extraction tests
6. Versioned atomic install + reuse tests
7. Render config to external config path
8. Validate subprocess with controlled env + timeout
9. Full orchestration tests
10. Documentation + verification

## Completion Summary

- Changed: Plan only; implementation not started.
- Verification: Plan reviewed against `AGENTS.md`, `.harness/AGENTS.md`, and
  `docs/ARCHITECTURE.md`.
- Known limitations: Exact Collector version, artifact URLs, checksums, and
  archive format must be selected before implementation.
- Follow-up: Approve the pinned artifact source and execute this plan before
  beginning service lifecycle work.
