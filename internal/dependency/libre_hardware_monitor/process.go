package librehardwaremonitor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// processOutputRunner executes one process and returns its combined output.
type processOutputRunner func(
	context.Context,
	processCommand,
) ([]byte, error)

// runOSProcess executes one command and discards successful standard output.
func runOSProcess(
	ctx context.Context,
	command processCommand,
) error {
	_, err := runOSProcessOutput(ctx, command)

	return err
}

// runOSProcessOutput executes one command and returns combined standard output.
// Failures retain executable, arguments, exit status, and diagnostics.
func runOSProcessOutput(
	ctx context.Context,
	command processCommand,
) ([]byte, error) {
	if strings.TrimSpace(command.Executable) == "" {
		return nil, fmt.Errorf(
			"Libre Hardware Monitor process executable is required",
		)
	}

	process := exec.CommandContext(
		ctx,
		command.Executable,
		command.Args...,
	)

	output, err := process.CombinedOutput()
	if err == nil {
		return output, nil
	}

	diagnostics := strings.TrimSpace(string(output))

	if diagnostics == "" {
		return nil, fmt.Errorf(
			"run process %q with arguments %#v: %w",
			command.Executable,
			command.Args,
			err,
		)
	}

	return nil, fmt.Errorf(
		"run process %q with arguments %#v: %w: %s",
		command.Executable,
		command.Args,
		err,
		diagnostics,
	)
}
