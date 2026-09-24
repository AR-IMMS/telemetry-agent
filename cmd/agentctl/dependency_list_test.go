package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestRunDependencyListsAvailableDependencies(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"list"},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{
					{
						Name:        "libre-hardware-monitor",
						DisplayName: "Libre Hardware Monitor",
						Description: "Collects Windows hardware metrics.",
					},
					{
						Name:        "windows-exporter",
						DisplayName: "Windows Exporter",
						Description: "Collects Windows host metrics.",
					},
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf("runDependency() exit code = %d, want 0", exitCode)
	}

	wantOutput := "" +
		"Available dependencies:\n" +
		"- libre-hardware-monitor: Libre Hardware Monitor — " +
		"Collects Windows hardware metrics.\n" +
		"- windows-exporter: Windows Exporter — " +
		"Collects Windows host metrics.\n"

	if stdout.String() != wantOutput {
		t.Fatalf(
			"runDependency() stdout = %q, want %q",
			stdout.String(),
			wantOutput,
		)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runDependency() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDependencyReportsNoAvailableDependencies(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"list"},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf("runDependency() exit code = %d, want 0", exitCode)
	}

	wantOutput := "No dependencies are available for this operating system.\n"

	if stdout.String() != wantOutput {
		t.Fatalf(
			"runDependency() stdout = %q, want %q",
			stdout.String(),
			wantOutput,
		)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runDependency() stderr = %q, want empty", stderr.String())
	}
}

func TestRunDependencyReportsListFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"list"},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return nil, errors.New("catalog unavailable")
			},
		},
	)

	if exitCode != 1 {
		t.Fatalf("runDependency() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runDependency() stdout = %q, want empty", stdout.String())
	}

	wantError := "list dependencies: catalog unavailable\n"
	if stderr.String() != wantError {
		t.Fatalf(
			"runDependency() stderr = %q, want %q",
			stderr.String(),
			wantError,
		)
	}
}
