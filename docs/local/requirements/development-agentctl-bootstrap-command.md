# Requirement: Development `agentctl bootstrap` Command

## Objective

Add a development-only CLI command that executes the completed bootstrap flow
against local configuration fragments and a pinned OpenTelemetry Collector.

The command must install or reuse the Collector, render the platform-specific
configuration, validate it, and activate the final `otel.yaml`.

## Command

```bash
go run ./cmd/agentctl bootstrap \
  --config-root ./configs \
  --install-dir ./tmp/agent-install \
  --config-path ./tmp/agent-config/otel.yaml
```

## Required Flags

| Flag                    | Required | Default          | Description                                                |
| ----------------------- | -------: | ---------------- | ---------------------------------------------------------- |
| `--config-root`         |      Yes | —                | Root directory containing configuration fragments.         |
| `--install-dir`         |      Yes | —                | Local Collector installation root.                         |
| `--config-path`         |      Yes | —                | Final rendered Collector configuration path.               |
| `--validation-endpoint` |       No | `127.0.0.1:4317` | Non-secret endpoint supplied only to Collector validation. |
| `--timeout`             |       No | `2m`             | Collector validation timeout.                              |

Unknown flags, missing required flags, and unsupported subcommands must return
a non-zero exit code and a concise usage error.

## Configuration Layers

The command must call `identity.CollectPlatformInfo()` and render layers in
this exact order:

```text
<config-root>/base/otel.yaml
<config-root>/profiles/laptop.yaml
<config-root>/os/<detected-os>/otel.yaml
```

Supported OS values are currently:

```text
linux
windows
```

The command must pass the selected layers to `config.RenderInput` and call
`bootstrap.Run()` with:

```text
OTEL_GATEWAY_ENDPOINT=<validation-endpoint>
```

This value is used only by `otelcol-contrib validate`. It must not be written
as a production endpoint or logged as a secret.

## Success Output

On success, print concise human-readable output containing:

```text
Collector version
Collector binary path
Final configuration path
Whether the Collector installation was reused
```

## Failure Rules

The command must:

- fail before download when configuration layer rendering fails;
- surface bootstrap errors without hiding their context;
- not start the Collector;
- not register a Windows Service or systemd unit;
- not contact the gateway;
- not create certificates or registration data.

## Files

```text
cmd/agentctl/
  main.go
  main_test.go
```

`main.go` is composition only: parse arguments, select layers, construct
options, invoke bootstrap, and print the result.

## Tests

Tests must not download a real artifact or execute a real Collector binary.

Required test cases:

1. `bootstrap` resolves base, laptop, and detected OS layers in order.
2. Linux resolves `os/linux/otel.yaml`.
3. Windows resolves `os/windows/otel.yaml`.
4. Unsupported subcommand returns a usage error.
5. Missing each required flag returns a usage error.
6. Unknown flag returns a usage error.
7. Default validation endpoint is `127.0.0.1:4317`.
8. Explicit validation endpoint overrides the default.
9. The command passes the selected layers and validation environment to the
   bootstrap dependency.
10. Bootstrap failure returns a non-zero exit code and does not print success
    output.
11. Bootstrap success prints the expected installation result.

## Acceptance Criteria

```bash
go test ./cmd/agentctl -v
go test ./...
go vet ./...
go build ./...
```

All commands must pass before this development command is considered complete.
