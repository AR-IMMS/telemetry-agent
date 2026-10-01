package agentinstallation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

const systemdUnitDirectory = "/etc/systemd/system"

type systemdCommand struct {
	Executable string
	Args       []string
}

type systemdRunner func(context.Context, systemdCommand) error

type systemdServiceInstaller struct {
	unitDirectory string
	writeFile     func(string, []byte, os.FileMode) error
	run           systemdRunner
}

func newSystemdServiceInstaller() systemdServiceInstaller {
	return systemdServiceInstaller{
		unitDirectory: systemdUnitDirectory,
		writeFile:     os.WriteFile,
		run: func(ctx context.Context, command systemdCommand) error {
			return exec.CommandContext(
				ctx,
				command.Executable,
				command.Args...,
			).Run()
		},
	}
}

// Install writes the Agent systemd unit, reloads systemd, then enables and
// restarts the service so a reinstallation uses the staged Agent binary.
func (i systemdServiceInstaller) Install(
	ctx context.Context,
	layout Layout,
) error {
	if ctx == nil {
		return fmt.Errorf("systemd installation context is required")
	}
	if strings.TrimSpace(i.unitDirectory) == "" {
		return fmt.Errorf("systemd unit directory is required")
	}
	if i.writeFile == nil {
		return fmt.Errorf("systemd unit writer is required")
	}
	if i.run == nil {
		return fmt.Errorf("systemd command runner is required")
	}

	unit, err := RenderSystemdUnit(layout)
	if err != nil {
		return err
	}

	serviceName := strings.TrimSpace(layout.ServiceName)
	if serviceName == "" {
		return fmt.Errorf("systemd service name is required")
	}
	if filepath.Base(serviceName) != serviceName {
		return fmt.Errorf(
			"systemd service name must not contain a path: %q",
			serviceName,
		)
	}

	unitName := serviceName + ".service"
	unitPath := path.Join(i.unitDirectory, unitName)

	if err := i.writeFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write systemd unit %q: %w", unitPath, err)
	}

	if err := i.run(ctx, systemdCommand{
		Executable: "systemctl",
		Args:       []string{"daemon-reload"},
	}); err != nil {
		return fmt.Errorf("reload systemd daemon: %w", err)
	}

	if err := i.run(ctx, systemdCommand{
		Executable: "systemctl",
		Args:       []string{"enable", unitName},
	}); err != nil {
		return fmt.Errorf(
			"enable systemd service %q: %w",
			unitName,
			err,
		)
	}

	if err := i.run(ctx, systemdCommand{
		Executable: "systemctl",
		Args:       []string{"restart", unitName},
	}); err != nil {
		return fmt.Errorf(
			"restart systemd service %q: %w",
			unitName,
			err,
		)
	}

	return nil
}
