package nodeexporter

import (
	"context"
	"fmt"
	"strings"
)

// systemdOutputRunner runs a systemctl command and returns its standard output.
type systemdOutputRunner func(
	context.Context,
	systemdCommand,
) ([]byte, error)

// systemdUnitEnabled reports whether the Agent-owned unit is enabled at boot.
func systemdUnitEnabled(
	ctx context.Context,
	serviceName string,
	run systemdOutputRunner,
) (bool, error) {
	serviceName = strings.TrimSpace(serviceName)

	if serviceName == "" {
		return false, fmt.Errorf(
			"Node Exporter systemd service name is required",
		)
	}
	if run == nil {
		return false, fmt.Errorf(
			"Node Exporter systemd output runner is required",
		)
	}

	output, err := run(ctx, systemdCommand{
		Executable: "systemctl",
		Args: []string{
			"show",
			"--property=UnitFileState",
			"--value",
			serviceName,
		},
	})
	if err != nil {
		return false, fmt.Errorf(
			"read Node Exporter systemd unit state: %w",
			err,
		)
	}

	switch strings.TrimSpace(string(output)) {
	case "enabled":
		return true, nil

	case "disabled":
		return false, nil

	default:
		return false, fmt.Errorf(
			"unexpected Node Exporter systemd unit state %q",
			strings.TrimSpace(string(output)),
		)
	}
}
