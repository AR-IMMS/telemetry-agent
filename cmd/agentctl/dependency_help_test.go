package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRunDependencyHelpWritesCommandSummary(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"help"},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 0 {
		t.Fatalf("runDependency(help) exit code = %d, want 0", exitCode)
	}

	const wantStdout = `Usage:
  agentctl dependency <command>

Commands:
  list                       List dependencies available on this operating system.
  install [dependency-name]  Install by name or select in a terminal.
  help                       Show this help.
`

	if stdout.String() != wantStdout {
		t.Fatalf(
			"dependency help stdout = %q, want %q",
			stdout.String(),
			wantStdout,
		)
	}

	if stderr.Len() != 0 {
		t.Fatalf(
			"dependency help stderr = %q, want empty",
			stderr.String(),
		)
	}
}

func TestRunDependencyWithoutCommandWritesHelpToStandardError(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		nil,
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf(
			"runDependency() exit code = %d, want 2",
			exitCode,
		)
	}

	if stdout.Len() != 0 {
		t.Fatalf(
			"runDependency() stdout = %q, want empty",
			stdout.String(),
		)
	}

	const wantStderr = `usage error: dependency command is required

Usage:
  agentctl dependency <command>

Commands:
  list                       List dependencies available on this operating system.
  install [dependency-name]  Install by name or select in a terminal.
  help                       Show this help.
`

	if stderr.String() != wantStderr {
		t.Fatalf(
			"runDependency() stderr = %q, want %q",
			stderr.String(),
			wantStderr,
		)
	}
}

func TestRunDependencyWithUnknownCommandWritesHelpToStandardError(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"remove"},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf(
			"runDependency(remove) exit code = %d, want 2",
			exitCode,
		)
	}

	if stdout.Len() != 0 {
		t.Fatalf(
			"runDependency(remove) stdout = %q, want empty",
			stdout.String(),
		)
	}

	const wantStderr = `usage error: unknown dependency command "remove"

Usage:
  agentctl dependency <command>

Commands:
  list                       List dependencies available on this operating system.
  install [dependency-name]  Install by name or select in a terminal.
  help                       Show this help.
`

	if stderr.String() != wantStderr {
		t.Fatalf(
			"runDependency(remove) stderr = %q, want %q",
			stderr.String(),
			wantStderr,
		)
	}
}

func TestRunDependencyInstallWithExtraArgumentsWritesHelpToStandardError(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"install", "node-exporter", "extra"},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf(
			"runDependency(install extra) exit code = %d, want 2",
			exitCode,
		)
	}

	if stdout.Len() != 0 {
		t.Fatalf(
			"runDependency(install extra) stdout = %q, want empty",
			stdout.String(),
		)
	}

	const wantStderr = `usage error: dependency install accepts exactly one dependency name

Usage:
  agentctl dependency <command>

Commands:
  list                       List dependencies available on this operating system.
  install [dependency-name]  Install by name or select in a terminal.
  help                       Show this help.
`

	if stderr.String() != wantStderr {
		t.Fatalf(
			"runDependency(install extra) stderr = %q, want %q",
			stderr.String(),
			wantStderr,
		)
	}
}

func TestRunDependencyListWithArgumentsWritesHelpToStandardError(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"list", "extra"},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf(
			"runDependency(list extra) exit code = %d, want 2",
			exitCode,
		)
	}

	if stdout.Len() != 0 {
		t.Fatalf(
			"runDependency(list extra) stdout = %q, want empty",
			stdout.String(),
		)
	}

	const wantStderr = `usage error: dependency list does not accept arguments

Usage:
  agentctl dependency <command>

Commands:
  list                       List dependencies available on this operating system.
  install [dependency-name]  Install by name or select in a terminal.
  help                       Show this help.
`

	if stderr.String() != wantStderr {
		t.Fatalf(
			"runDependency(list extra) stderr = %q, want %q",
			stderr.String(),
			wantStderr,
		)
	}
}
