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

// newOSSystemdOutputRunner creates the production systemd output runner.
func newOSSystemdOutputRunner() systemdOutputRunner {
	runner := osSystemdRunner{
		newCommand: func(
			ctx context.Context,
			executable string,
			args ...string,
		) systemdProcess {
			return exec.CommandContext(ctx, executable, args...)
		},
	}

	return runner.Output
}

// Run executes one systemd command and preserves its bounded diagnostics.
func (r osSystemdRunner) Run(
	ctx context.Context,
	command systemdCommand,
) error {
	_, err := r.Output(ctx, command)

	return err
}

// Output executes one systemd command and returns its standard output.
func (r osSystemdRunner) Output(
	ctx context.Context,
	command systemdCommand,
) ([]byte, error) {
	if strings.TrimSpace(command.Executable) == "" {
		return nil, fmt.Errorf(
			"systemd command executable is required",
		)
	}
	if r.newCommand == nil {
		return nil, fmt.Errorf(
			"systemd command factory is required",
		)
	}

	output, err := r.newCommand(
		ctx,
		command.Executable,
		command.Args...,
	).CombinedOutput()
	if err == nil {
		return output, nil
	}

	diagnostics := strings.TrimSpace(string(output))

	if diagnostics == "" {
		return nil, fmt.Errorf(
			"run systemd command %q with arguments %#v: %w",
			command.Executable,
			command.Args,
			err,
		)
	}

	return nil, fmt.Errorf(
		"run systemd command %q with arguments %#v: %w: %s",
		command.Executable,
		command.Args,
		err,
		diagnostics,
	)
}
