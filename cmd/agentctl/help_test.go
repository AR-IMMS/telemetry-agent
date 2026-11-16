package main

import (
	"bytes"
	"context"
	"strings"
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
  integration Manage desired Integration instances and local convergence diagnostics.
  monitoring  Manage monitoring instances, templates, and proposals.
  run         Run the OpenTelemetry Collector under supervision.
  inventory   Read persisted resource discovery inventory.
  discovery   Review and approve durable monitoring proposals.
  status      Show live Agent health.
  doctor      Explain available health evidence and unknown dimensions.
  enroll      Bind this Agent installation to an AR-IMMS Core node.
  install     Install and start the Agent as a managed service.
  uninstall   Safely remove the Agent and its managed dependencies.
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
  integration Manage desired Integration instances and local convergence diagnostics.
  monitoring  Manage monitoring instances, templates, and proposals.
  run         Run the OpenTelemetry Collector under supervision.
  inventory   Read persisted resource discovery inventory.
  discovery   Review and approve durable monitoring proposals.
  status      Show live Agent health.
  doctor      Explain available health evidence and unknown dimensions.
  enroll      Bind this Agent installation to an AR-IMMS Core node.
  install     Install and start the Agent as a managed service.
  uninstall   Safely remove the Agent and its managed dependencies.
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
  integration Manage desired Integration instances and local convergence diagnostics.
  monitoring  Manage monitoring instances, templates, and proposals.
  run         Run the OpenTelemetry Collector under supervision.
  inventory   Read persisted resource discovery inventory.
  discovery   Review and approve durable monitoring proposals.
  status      Show live Agent health.
  doctor      Explain available health evidence and unknown dimensions.
  enroll      Bind this Agent installation to an AR-IMMS Core node.
  install     Install and start the Agent as a managed service.
  uninstall   Safely remove the Agent and its managed dependencies.
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

func TestCobraHelpExposesApprovedMonitoringHierarchy(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "root help flag",
			args: []string{"--help"},
			want: []string{"monitoring", "Manage monitoring instances, templates, and proposals"},
		},
		{
			name: "monitoring subtree",
			args: []string{"monitoring", "--help"},
			want: []string{"instance", "template", "proposal"},
		},
		{
			name: "instance actions",
			args: []string{"monitoring", "instance", "--help"},
			want: []string{"configure", "update", "enable", "disable", "delete", "list", "inspect", "status", "history", "retry"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runForTest(t, test.args, dependencies{})
			if code != 0 || stderr != "" {
				t.Fatalf("run(%v) = (%d, stderr=%q), want successful help", test.args, code, stderr)
			}
			for _, want := range test.want {
				if !strings.Contains(stdout, want) {
					t.Errorf("help output %q does not contain %q", stdout, want)
				}
			}
		})
	}
}
