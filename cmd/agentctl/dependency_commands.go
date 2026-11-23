package main

import (
	"context"
	"fmt"
	"io"

	dependencycommands "github.com/ar-imms/telemetry-agent/internal/cli/commands/dependency"
)

func runDependencyList(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {
	if len(args) != 1 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency list does not accept arguments",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	return dependencycommands.List(ctx, stdout, stderr, deps.listDependencies)
}
