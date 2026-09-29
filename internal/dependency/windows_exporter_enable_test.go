package dependency

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestWindowsExporterEnablerStartsOwnedService(
	t *testing.T,
) {
	var commands []processCommand

	enabler := windowsExporterEnabler{
		options: DefaultWindowsExporterOptions(),
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			_ context.Context,
			command processCommand,
		) error {
			commands = append(commands, command)

			return nil
		},
	}

	err := enabler.Enable(
		context.Background(),
		[]agentstate.OwnedResource{
			{
				Kind:       "windows-service",
				Identifier: windowsExporterServiceName,
			},
		},
	)
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(commands))
	}

	command := commands[0]
	if command.Executable != "powershell.exe" {
		t.Fatalf(
			"command executable = %q, want powershell.exe",
			command.Executable,
		)
	}

	script := command.Args[len(command.Args)-1]
	for _, want := range []string{
		"Get-Service -Name 'windows_exporter'",
		"Set-Service -Name 'windows_exporter' -StartupType Automatic",
		"Start-Service -Name 'windows_exporter'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("PowerShell script = %q, want %q", script, want)
		}
	}
}

func TestWindowsExporterEnablerRejectsMissingOwnedService(
	t *testing.T,
) {
	called := false

	enabler := windowsExporterEnabler{
		options: DefaultWindowsExporterOptions(),
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			context.Context,
			processCommand,
		) error {
			called = true

			return nil
		},
	}

	err := enabler.Enable(
		context.Background(),
		[]agentstate.OwnedResource{
			{
				Kind:       "config-file",
				Identifier: enabler.options.ConfigPath,
			},
		},
	)

	if err == nil {
		t.Fatal("Enable() error = nil, want ownership error")
	}
	if called {
		t.Fatal("process runner was called without owned Windows service")
	}
}

func TestWindowsExporterEnablerRejectsNilContext(
	t *testing.T,
) {
	called := false

	enabler := windowsExporterEnabler{
		options: DefaultWindowsExporterOptions(),
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			context.Context,
			processCommand,
		) error {
			called = true

			return nil
		},
	}

	err := enabler.Enable(
		nil,
		[]agentstate.OwnedResource{
			{
				Kind:       "windows-service",
				Identifier: windowsExporterServiceName,
			},
		},
	)

	if err == nil {
		t.Fatal("Enable() error = nil, want nil-context error")
	}
	if called {
		t.Fatal("process runner was called with nil context")
	}
}
