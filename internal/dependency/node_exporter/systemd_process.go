package nodeexporter

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// systemdProcess is the minimal process contract needed by the systemd runner.
type systemdProcess interface {
	CombinedOutput() ([]byte, error)
}

// systemdCommandFactory creates a process for one systemd command.
type systemdCommandFactory func(
	context.Context,
	string,
	...string,
) systemdProcess

// osSystemdRunner executes systemctl commands through the operating system.
type osSystemdRunner struct {
	newCommand systemdCommandFactory
}

// newOSSystemdRunner creates the production systemd command runner.
func newOSSystemdRunner() systemdRunner {
	runner := osSystemdRunner{
		newCommand: func(
			ctx context.Context,
			executable string,
			args ...string,
		) systemdProcess {
			return exec.CommandContext(ctx, executable, args...)
		},
	}

	return runner.Run
}

// Run executes one systemd command and preserves its bounded diagnostics.
func (r osSystemdRunner) Run(
	ctx context.Context,
	command systemdCommand,
) error {
	if strings.TrimSpace(command.Executable) == "" {
		return fmt.Errorf("systemd command executable is required")
	}
	if r.newCommand == nil {
		return fmt.Errorf("systemd command factory is required")
	}

	output, err := r.newCommand(
		ctx,
		command.Executable,
		command.Args...,
	).CombinedOutput()

	if err == nil {
		return nil
	}

	diagnostics := strings.TrimSpace(string(output))

	if diagnostics == "" {
		return fmt.Errorf(
			"run systemd command %q with arguments %#v: %w",
			command.Executable,
			command.Args,
			err,
		)
	}

	return fmt.Errorf(
		"run systemd command %q with arguments %#v: %w: %s",
		command.Executable,
		command.Args,
		err,
		diagnostics,
	)
}
