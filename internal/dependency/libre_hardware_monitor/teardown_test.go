package librehardwaremonitor

import (
	"context"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestLHMTeardownDisablesOwnedScheduledTask(t *testing.T) {
	options := DefaultOptions()
	var commands []processCommand

	teardown := lhmTeardown{
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

	err := teardown.Teardown(
		context.Background(),
		agentstate.TeardownActionDisable,
		[]agentstate.OwnedResource{
			{
				Kind:       "scheduled-task",
				Identifier: options.TaskName,
			},
		},
	)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
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
		"Get-ScheduledTask -TaskName '" + options.TaskName + "'",
		"Stop-ScheduledTask -TaskName '" + options.TaskName + "'",
		"Disable-ScheduledTask -TaskName '" + options.TaskName + "'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("PowerShell script = %q, want %q", script, want)
		}
	}
}

func TestLHMTeardownUninstallsOwnedResources(t *testing.T) {
	options := DefaultOptions()
	var steps []string

	teardown := lhmTeardown{
		options: options,
		administrator: func() (bool, error) {
			return true, nil
		},
		runProcess: func(
			_ context.Context,
			command processCommand,
		) error {
			script := command.Args[len(command.Args)-1]

			switch {
			case strings.Contains(
				script,
				"Disable-ScheduledTask -TaskName",
			):
				steps = append(steps, "disable-task")

			case strings.Contains(
				script,
				"Unregister-ScheduledTask -TaskName",
			):
				steps = append(steps, "remove-task-and-firewall")

			default:
				t.Fatalf("unexpected PowerShell script: %q", script)
			}

			return nil
		},
		removeDirectory: func(path string) error {
			if path != options.InstallDir {
				t.Fatalf(
					"installation directory = %q, want %q",
					path,
					options.InstallDir,
				)
			}

			steps = append(steps, "remove-directory")

			return nil
		},
	}

	err := teardown.Teardown(
		context.Background(),
		agentstate.TeardownActionUninstall,
		[]agentstate.OwnedResource{
			{
				Kind:       "scheduled-task",
				Identifier: options.TaskName,
			},
			{
				Kind:       "firewall-rule",
				Identifier: options.FirewallName,
			},
			{
				Kind:       "directory",
				Identifier: options.InstallDir,
			},
		},
	)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}

	got := strings.Join(steps, ",")
	if got != "disable-task,remove-task-and-firewall,remove-directory" {
		t.Fatalf(
			"teardown steps = %q, want disable-task,remove-task-and-firewall,remove-directory",
			got,
		)
	}
}
