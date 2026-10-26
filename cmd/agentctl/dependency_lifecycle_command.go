package main

import (
	"context"
	"fmt"
	"io"
)

func runDependencyLifecycle(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {

	command := args[0]
	action := dependencyLifecycleDisable

	if command == "uninstall" {
		action = dependencyLifecycleUninstall
	}

	names, statePath, err := parseDependencyInstallArguments(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "usage error: %v\n\n", err)
		writeDependencyHelp(stderr)

		return 2
	}

	if len(names) != 1 {
		fmt.Fprintf(
			stderr,
			"usage error: dependency %s accepts exactly one dependency name\n",
			command,
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	if deps.manageDependencyLifecycle == nil {
		fmt.Fprintf(
			stderr,
			"%s dependency: lifecycle manager is not configured\n",
			command,
		)

		return 1
	}

	if err := deps.manageDependencyLifecycle(
		ctx,
		names[0],
		statePath,
		action,
	); err != nil {
		fmt.Fprintf(stderr, "%s dependency: %v\n", command, err)

		return 1
	}

	fmt.Fprintf(
		stdout,
		"Dependency %s scheduled: %s\n",
		command,
		names[0],
	)

	return 0

}
