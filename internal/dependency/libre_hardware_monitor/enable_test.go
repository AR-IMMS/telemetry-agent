package librehardwaremonitor

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestLHMEnablerStartsOwnedScheduledTask(
	t *testing.T,
) {
	options := DefaultOptions()
	var commands []processCommand

	enabler := lhmEnabler{
		options: options,
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
				Kind:       "scheduled-task",
				Identifier: options.TaskName,
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
		"Get-ScheduledTask -TaskName",
		"Enable-ScheduledTask -TaskName",
		"Start-ScheduledTask -TaskName",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("PowerShell script = %q, want %q", script, want)
		}
	}
}

func TestLHMEnablerRejectsMissingOwnedScheduledTask(
	t *testing.T,
) {
	called := false
	options := DefaultOptions()

	enabler := lhmEnabler{
		options: options,
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
				Kind:       "directory",
				Identifier: options.InstallDir,
			},
		},
	)

	if err == nil {
		t.Fatal("Enable() error = nil, want ownership error")
	}
	if called {
		t.Fatal("process runner was called without owned scheduled task")
	}
}

func TestLHMEnablerRejectsNilContext(
	t *testing.T,
) {
	called := false
	options := DefaultOptions()

	enabler := lhmEnabler{
		options: options,
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
				Kind:       "scheduled-task",
				Identifier: options.TaskName,
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
