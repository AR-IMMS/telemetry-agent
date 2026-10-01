package agentinstallation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// systemdServiceRemover removes one Agent-owned systemd service and its unit.
type systemdServiceRemover struct {
	run        func(context.Context, systemdCommand) error
	removeFile func(string) error
}

func (r systemdServiceRemover) Remove(
	ctx context.Context,
	serviceName string,
	unitPath string,
) error {
	if ctx == nil {
		return fmt.Errorf("systemd removal context is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("systemd service name is required")
	}
	if strings.TrimSpace(unitPath) == "" {
		return fmt.Errorf("systemd unit path is required")
	}
	if r.run == nil {
		return fmt.Errorf("systemd command runner is required")
	}
	if r.removeFile == nil {
		return fmt.Errorf("systemd unit remover is required")
	}

	if err := r.run(ctx, systemdCommand{
		Executable: "systemctl",
		Args: []string{
			"disable",
			"--now",
			serviceName + ".service",
		},
	}); err != nil {
		return fmt.Errorf(
			"disable Agent systemd service %q: %w",
			serviceName,
			err,
		)
	}

	if err := r.removeFile(unitPath); err != nil {
		return fmt.Errorf(
			"remove Agent systemd unit %q: %w",
			unitPath,
			err,
		)
	}

	if err := r.run(ctx, systemdCommand{
		Executable: "systemctl",
		Args:       []string{"daemon-reload"},
	}); err != nil {
		return fmt.Errorf("reload systemd units: %w", err)
	}

	return nil
}

func newSystemdServiceRemover() systemdServiceRemover {
	return systemdServiceRemover{
		run: func(ctx context.Context, command systemdCommand) error {
			return exec.CommandContext(
				ctx,
				command.Executable,
				command.Args...,
			).Run()
		},
		removeFile: os.Remove,
	}
}
