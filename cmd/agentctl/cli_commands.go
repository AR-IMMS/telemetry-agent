package main

import (
	"context"
	"io"

	agentcli "github.com/ar-imms/telemetry-agent/internal/cli"
)

// executeAgentctl builds the command tree while forwarding every existing command
// to its compatibility handler. The new Monitoring paths are aliases only.
func executeAgentctl(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	return agentcli.Execute(ctx, args, stdout, stderr, func(commandContext context.Context, commandArgs []string, commandStdout, commandStderr io.Writer) int {
		return runLegacy(commandContext, commandArgs, commandStdout, commandStderr, deps)
	}, writeRootHelp)
}
