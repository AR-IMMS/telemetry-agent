package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

func runDependencyInstall(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {
	names, statePath, err := parseDependencyInstallArguments(
		args[1:],
	)
	if err != nil {
		fmt.Fprintf(stderr, "usage error: %v\n\n", err)
		writeDependencyHelp(stderr)

		return 2
	}
	if len(names) == 0 {
		if deps.listDependencies == nil {
			fmt.Fprintln(
				stderr,
				"install dependency: lister is not configured",
			)

			return 1
		}

		definitions, err := deps.listDependencies(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "install dependency: %v\n", err)

			return 1
		}

		if deps.selectDependencies == nil {
			fmt.Fprintln(
				stderr,
				"install dependency: selector is not configured",
			)

			return 1
		}

		names, err = deps.selectDependencies(
			ctx,
			definitions,
			stdout,
		)
		if errors.Is(err, errDependencySelectionCancelled) {
			fmt.Fprintln(stdout, "Dependency installation cancelled.")

			return 0
		}
		if err != nil {
			fmt.Fprintf(stderr, "install dependency: %v\n", err)

			return 1
		}
	} else if len(names) != 1 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency install accepts exactly one dependency name",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}
	if deps.manageDependency == nil &&
		deps.installDependency == nil {
		fmt.Fprintln(
			stderr,
			"install dependency: installer is not configured",
		)

		return 1
	}
	for _, name := range names {
		if deps.manageDependency != nil {
			result, err := deps.manageDependency(
				ctx,
				name,
				statePath,
			)
			if err != nil {
				fmt.Fprintf(stderr, "install dependency: %v\n", err)

				return 1
			}

			fmt.Fprintf(
				stdout,
				"Dependency installed: %s\n",
				result.Name,
			)

			continue
		}

		result, err := deps.installDependency(ctx, name)
		if err != nil {
			fmt.Fprintf(stderr, "install dependency: %v\n", err)

			return 1
		}

		fmt.Fprintf(stdout, "Dependency installed: %s\n", result.Name)
	}
	return 0

}
