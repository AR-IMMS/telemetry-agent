# Cross-platform CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an OS-native GitHub Actions PR gate for the supported Windows and Linux builds.

**Architecture:** A single workflow runs the same Go quality gates independently on native Linux and Windows runners. The workflow has read-only permissions, bounded execution, concurrency cancellation, and no release side effects.

**Tech Stack:** GitHub Actions, Go 1.25.5 as declared in `go.mod`, Ubuntu and Windows hosted runners.

**Spec:** `docs/superpowers/specs/2026-10-26-cross-platform-ci.md`

## Global Constraints

- Initial supported platforms: Windows and Linux.
- Run `go test ./... -count=1`, `go vet ./...`, and `go build ./...`.
- Keep workflow permissions read-only and do not publish artifacts.

## Review Focus

- Pull requests targeting either supported development branch must trigger the same checks.
- Windows-specific and Linux-specific Go files must each compile and run their tests on native runners.
- Cancelled or timed-out jobs must not leave an unbounded CI run.

---

### Task 1: Add the cross-platform PR workflow

**Files:**
- Create: `.github/workflows/ci.yml`
- Spec: `docs/superpowers/specs/2026-10-26-cross-platform-ci.md`

**Interfaces:**
- Consumes: Go module version from `go.mod` and the repository's existing Go test/build commands.
- Produces: `CI` workflow for pushes and pull requests on `main` and `dev`.

- [x] Add triggers for pushes and pull requests targeting `main` and `dev`, with concurrency cancellation and read-only permissions.
- [x] Add a native runner matrix for `ubuntu-latest` and `windows-latest`; set `fail-fast: false` and a 30-minute job timeout.
- [x] Set up Go from `go.mod`, enable module caching keyed by `go.sum`, then run tests, vet, and build on each native runner.
- [x] Validate workflow syntax with `actionlint`; confirm required triggers, runners, commands, permissions, and timeout settings.
- [x] Run `go test ./... -count=1`, `go vet ./...`, and `go build ./...` locally on Windows. Cross-builds for Linux amd64 and Windows amd64 passed; native Linux execution remains for GitHub-hosted CI.
