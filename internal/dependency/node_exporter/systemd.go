package nodeexporter

import (
	"context"
	"fmt"
	"strings"
)

// systemdCommand describes one systemctl invocation without executing it.
type systemdCommand struct {
	Executable string
	Args       []string
}

// systemdRunner executes one systemd command. It is injected for unit tests.
type systemdRunner func(context.Context, systemdCommand) error

// EnableAndStartSystemdService reloads unit definitions, enables the service,
// and starts it immediately.
func EnableAndStartSystemdService(
	ctx context.Context,
	serviceName string,
	run systemdRunner,
) error {
	serviceName = strings.TrimSpace(serviceName)

	if serviceName == "" {
		return fmt.Errorf("Node Exporter systemd service name is required")
	}
	if run == nil {
		return fmt.Errorf("Node Exporter systemd runner is required")
	}

	commands := []systemdCommand{
		{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
		{
			Executable: "systemctl",
			Args: []string{
				"enable",
				"--now",
				serviceName,
			},
		},
	}

	for _, command := range commands {
		if err := run(ctx, command); err != nil {
			return fmt.Errorf(
				"run Node Exporter systemd command %#v: %w",
				command.Args,
				err,
			)
		}
	}

	return nil
}
