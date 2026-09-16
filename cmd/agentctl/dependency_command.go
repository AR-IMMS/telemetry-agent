package main

import (
	"context"
	"fmt"
	"io"
)

func runDependency(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	if len(args) != 2 || args[0] != "install" {
		fmt.Fprintln(
			stderr,
			"usage: agentctl dependency install <dependency-name>",
		)
		return 2
	}

	if deps.installDependency == nil {
		fmt.Fprintln(stderr, "install dependency: installer is not configured")
		return 1
	}

	result, err := deps.installDependency(ctx, args[1])
	if err != nil {
		fmt.Fprintf(stderr, "install dependency: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Dependency installed: %s\n", result.Name)

	return 0
}
