# Cross-platform CI Workflow Design

## Goal

Add a focused GitHub Actions pull-request gate that proves the Agent tests and builds on the supported Windows and Linux platforms.

## Workflow

- Run on pull requests targeting `main` or `dev` and pushes to either branch.
- Use native `ubuntu-latest` and `windows-latest` runners.
- On each runner, use the Go version declared in `go.mod`, cache Go modules, and run `go test ./... -count=1`, `go vet ./...`, and `go build ./...`.
- Give jobs a bounded timeout, allow both OS jobs to report independently, cancel superseded runs, and grant only `contents: read` token permission.
- Do not build or publish release artifacts; packaging and release workflows are outside the current repository scope.

## Rationale

The repository targets Windows and Linux and currently has no CI workflow or release packaging scripts. Native runners exercise platform-specific files and commands. The Datadog Agent's public repository uses substantial custom CI/build tooling; this project borrows the staged, platform-aware validation idea without importing that machinery.

## Acceptance

- The workflow parses as valid GitHub Actions YAML.
- Trigger filters include pushes and pull requests for `main` and `dev`.
- Both native OS jobs run tests, vet, and build with the `go.mod` toolchain.
- Local equivalents of all three commands pass before delivery.
