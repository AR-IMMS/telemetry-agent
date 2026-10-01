package agentinstallation

import (
	"context"
	"reflect"
	"testing"
)

func TestSystemdServiceRemoverDisablesRemovesUnitAndReloads(
	t *testing.T,
) {
	var steps []string
	var commands []systemdCommand

	remover := systemdServiceRemover{
		run: func(
			_ context.Context,
			command systemdCommand,
		) error {
			commands = append(commands, command)
			steps = append(steps, "systemctl "+command.Args[0])

			return nil
		},
		removeFile: func(path string) error {
			if path !=
				"/etc/systemd/system/ar-imms-telemetry-agent.service" {
				t.Fatalf("unit path = %q", path)
			}

			steps = append(steps, "remove-unit")

			return nil
		},
	}

	err := remover.Remove(
		context.Background(),
		"ar-imms-telemetry-agent",
		"/etc/systemd/system/ar-imms-telemetry-agent.service",
	)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if got, want := steps,
		[]string{
			"systemctl disable",
			"remove-unit",
			"systemctl daemon-reload",
		}; !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}

	if got, want := commands, []systemdCommand{
		{
			Executable: "systemctl",
			Args: []string{
				"disable",
				"--now",
				"ar-imms-telemetry-agent.service",
			},
		},
		{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}
