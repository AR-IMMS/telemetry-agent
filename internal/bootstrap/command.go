package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run executes a Collector command with bounded output capture.
func (OSCommandRunner) Run(
	ctx context.Context,
	command Command,
) CommandResult {
	if command.Executable == "" {
		return CommandResult{
			ExitCode: -1,
			Err:      fmt.Errorf("command executable is required"),
		}
	}

	process := exec.CommandContext(
		ctx,
		command.Executable,
		command.Args...,
	)

	if len(command.Environment) > 0 {
		// Preserve the host environment while allowing validation-only overrides.
		process.Env = append(os.Environ(), command.Environment...)
	}

	var stdout, stderr bytes.Buffer

	stdoutBuffer := &limitedBuffer{buffer: &stdout}
	stderrBuffer := &limitedBuffer{buffer: &stderr}

	process.Stdout = stdoutBuffer
	process.Stderr = stderrBuffer

	err := process.Run()

	result := CommandResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		OutputTruncated: stdoutBuffer.truncated || stderrBuffer.truncated,
		Err:             err,
	}

	switch {
	case err == nil:
		result.ExitCode = 0

	case ctx.Err() != nil:
		result.ExitCode = -1

	case isExitError(err):
		result.ExitCode = err.(*exec.ExitError).ExitCode()

	default:
		result.ExitCode = -1
	}

	return result
}

func isExitError(err error) bool {
	// Keep process-start failures distinct from commands that returned an exit code.
	_, ok := err.(*exec.ExitError)
	return ok
}

type limitedBuffer struct {
	buffer    *bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)

	remaining := maxCommandOutput - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return originalLength, nil
	}

	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}

	if _, err := b.buffer.Write(p); err != nil {
		return 0, err
	}

	// Return originalLength: otherwise os/exec may treat this as short write.
	return originalLength, nil
}

func validateCollector(
	ctx context.Context,
	runner CommandRunner,
	binaryPath string,
	configPath string,
	environment []string,
) error {
	// Validation is deliberately delegated to the Collector so its own config
	// parser and component validation remain the source of truth.
	if runner == nil {
		return fmt.Errorf("Collector command runner is required")
	}

	result := runner.Run(ctx, Command{
		Executable:  binaryPath,
		Args:        []string{"validate", "--config", configPath},
		Environment: environment,
	})

	if ctx.Err() != nil {
		// Context cancellation takes precedence so timeout failures remain actionable.
		return fmt.Errorf(
			"Collector configuration validation timed out: %w",
			ctx.Err(),
		)
	}

	if result.Err == nil && result.ExitCode == 0 {
		return nil
	}

	details := commandOutput(result)
	if details == "" {
		details = "no Collector output was captured"
	}

	return fmt.Errorf(
		"Collector configuration validation failed (exit %d): %s",
		result.ExitCode,
		details,
	)
}

func commandOutput(result CommandResult) string {
	// Prefer stderr, but retain stdout because Collector diagnostics may use either.
	parts := make([]string, 0, 2)

	if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
		parts = append(parts, stderr)
	}

	if stdout := strings.TrimSpace(result.Stdout); stdout != "" {
		parts = append(parts, stdout)
	}

	output := strings.Join(parts, "\n")

	if result.OutputTruncated {
		output += "\n[Collector output truncated]"
	}

	return output
}
