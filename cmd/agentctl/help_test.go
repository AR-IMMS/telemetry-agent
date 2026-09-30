package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRunHelpWritesCommandSummary(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"help"},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 0 {
		t.Fatalf("run(help) exit code = %d, want 0", exitCode)
	}

	const wantStdout = `Usage:
  agentctl <command> [options]

Commands:
  bootstrap   Bootstrap the OpenTelemetry Collector.
  dependency  Manage telemetry dependencies.
  run         Run the OpenTelemetry Collector under supervision.
  status      Show live Agent health.
  install     Install and start the Agent as a managed service.
  help        Show this help.
`

	if stdout.String() != wantStdout {
		t.Fatalf(
			"help stdout = %q, want %q",
			stdout.String(),
			wantStdout,
		)
	}

	if stderr.Len() != 0 {
		t.Fatalf("help stderr = %q, want empty", stderr.String())
	}
}

func TestRunWithoutCommandWritesHelpToStandardError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		nil,
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}

	if stdout.Len() != 0 {
		t.Fatalf(
			"run() stdout = %q, want empty",
			stdout.String(),
		)
	}

	const wantStderr = `usage error: command is required

Usage:
  agentctl <command> [options]

Commands:
  bootstrap   Bootstrap the OpenTelemetry Collector.
  dependency  Manage telemetry dependencies.
  run         Run the OpenTelemetry Collector under supervision.
  status      Show live Agent health.
  install     Install and start the Agent as a managed service.
  help        Show this help.
`

	if stderr.String() != wantStderr {
		t.Fatalf(
			"run() stderr = %q, want %q",
			stderr.String(),
			wantStderr,
		)
	}
}

func TestRunWithUnknownCommandWritesHelpToStandardError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"deploy"},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf("run(deploy) exit code = %d, want 2", exitCode)
	}

	if stdout.Len() != 0 {
		t.Fatalf(
			"run(deploy) stdout = %q, want empty",
			stdout.String(),
		)
	}

	const wantStderr = `usage error: unknown command "deploy"

Usage:
  agentctl <command> [options]

Commands:
  bootstrap   Bootstrap the OpenTelemetry Collector.
  dependency  Manage telemetry dependencies.
  run         Run the OpenTelemetry Collector under supervision.
  status      Show live Agent health.
  install     Install and start the Agent as a managed service.
  help        Show this help.
`

	if stderr.String() != wantStderr {
		t.Fatalf(
			"run(deploy) stderr = %q, want %q",
			stderr.String(),
			wantStderr,
		)
	}
}
