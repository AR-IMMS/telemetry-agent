package dependency

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const maxProcessDiagnostics = 64 << 10

// runOSProcess executes one process and returns bounded diagnostics on failure.
func runOSProcess(
	ctx context.Context,
	command processCommand,
) error {
	if ctx == nil {
		return fmt.Errorf("process context is required")
	}
	if strings.TrimSpace(command.Executable) == "" {
		return fmt.Errorf("process executable is required")
	}

	process := exec.CommandContext(
		ctx,
		command.Executable,
		command.Args...,
	)

	var diagnostics boundedOutput
	process.Stdout = &diagnostics
	process.Stderr = &diagnostics

	if err := process.Run(); err != nil {
		output := strings.TrimSpace(diagnostics.String())

		if output == "" {
			return fmt.Errorf(
				"run process %q with arguments %#v: %w",
				command.Executable,
				command.Args,
				err,
			)
		}

		if diagnostics.truncated {
			output += "\n[process output truncated]"
		}

		return fmt.Errorf(
			"run process %q with arguments %#v: %w: %s",
			command.Executable,
			command.Args,
			err,
			output,
		)
	}

	return nil
}

// boundedOutput keeps child-process diagnostics from consuming unbounded memory.
type boundedOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

// Write records up to maxProcessDiagnostics bytes while allowing the child
// process to continue writing normally.
func (b *boundedOutput) Write(content []byte) (int, error) {
	originalLength := len(content)
	remaining := maxProcessDiagnostics - b.buffer.Len()

	if remaining <= 0 {
		b.truncated = true
		return originalLength, nil
	}

	if len(content) > remaining {
		content = content[:remaining]
		b.truncated = true
	}

	_, err := b.buffer.Write(content)

	return originalLength, err
}

// String returns the retained process diagnostics.
func (b *boundedOutput) String() string {
	return b.buffer.String()
}
