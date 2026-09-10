package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

func (OSCommandRunner) Run(ctx context.Context, executable string, args ...string) CommandResult {
	command := exec.CommandContext(ctx, executable, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &limitedBuffer{buffer: &stdout}, &limitedBuffer{buffer: &stderr}
	err := command.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	if err == nil {
		result.ExitCode = 0
	} else if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
	} else {
		result.ExitCode = -1
	}
	return result
}

type limitedBuffer struct{ buffer *bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := maxCommandOutput - b.buffer.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return b.buffer.Write(p)
}

func validateCollector(ctx context.Context, runner CommandRunner, binary, config string) error {
	result := runner.Run(ctx, binary, "validate", "--config", config)
	if result.Err != nil || result.ExitCode != 0 {
		return fmt.Errorf("Collector configuration validation failed (exit %d): %s", result.ExitCode, trimOutput(result.Stderr))
	}
	return nil
}

func trimOutput(output string) string {
	if len(output) > maxCommandOutput {
		return output[:maxCommandOutput]
	}
	return output
}
