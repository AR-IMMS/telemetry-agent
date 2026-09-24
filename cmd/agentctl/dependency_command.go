package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

func writeDependencyHelp(output io.Writer) {
	fmt.Fprint(output, `Usage:
  agentctl dependency <command>

Commands:
  list                       List dependencies available on this operating system.
  install [dependency-name]  Install by name or select in a terminal.
  help                       Show this help.
`)
}

func runDependency(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	if len(args) == 0 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency command is required",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	if len(args) == 1 && args[0] == "help" {
		writeDependencyHelp(stdout)

		return 0
	}

	if args[0] == "list" {
		if len(args) != 1 {
			fmt.Fprintln(
				stderr,
				"usage error: dependency list does not accept arguments",
			)
			fmt.Fprintln(stderr)
			writeDependencyHelp(stderr)

			return 2
		}

		if deps.listDependencies == nil {
			fmt.Fprintln(
				stderr,
				"list dependencies: lister is not configured",
			)

			return 1
		}

		definitions, err := deps.listDependencies(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "list dependencies: %v\n", err)

			return 1
		}

		if len(definitions) == 0 {
			fmt.Fprintln(
				stdout,
				"No dependencies are available for this operating system.",
			)

			return 0
		}

		fmt.Fprintln(stdout, "Available dependencies:")
		for _, definition := range definitions {
			fmt.Fprintf(
				stdout,
				"- %s: %s — %s\n",
				definition.Name,
				definition.DisplayName,
				definition.Description,
			)
		}

		return 0
	}

	if args[0] == "status" {
		if len(args) != 1 {
			fmt.Fprintln(
				stderr,
				"usage error: dependency status does not accept arguments",
			)
			fmt.Fprintln(stderr)
			writeDependencyHelp(stderr)

			return 2
		}

		if deps.listDependencyStatuses == nil {
			fmt.Fprintln(
				stderr,
				"dependency status: lister is not configured",
			)

			return 1
		}

		results, err := deps.listDependencyStatuses(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "dependency status: %v\n", err)

			return 1
		}

		fmt.Fprintln(stdout, "Dependency status:")

		for _, result := range results {
			if result.InspectionError != nil {
				fmt.Fprintf(
					stdout,
					"- %s: unknown (%v)\n",
					result.Definition.Name,
					result.InspectionError,
				)

				continue
			}

			fmt.Fprintf(
				stdout,
				"- %s: %s (%s)\n",
				result.Definition.Name,
				result.Status.Availability,
				result.Status.Health,
			)
		}

		return 0
	}

	if args[0] != "install" {
		fmt.Fprintf(
			stderr,
			"usage error: unknown dependency command %q\n\n",
			args[0],
		)
		writeDependencyHelp(stderr)

		return 2
	}

	var names []string

	if len(args) == 1 {
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
	} else if len(args) == 2 {
		names = []string{args[1]}
	} else {
		fmt.Fprintln(
			stderr,
			"usage error: dependency install accepts exactly one dependency name",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	if deps.installDependency == nil {
		fmt.Fprintln(stderr, "install dependency: installer is not configured")

		return 1
	}

	for _, name := range names {
		result, err := deps.installDependency(ctx, name)
		if err != nil {
			fmt.Fprintf(stderr, "install dependency: %v\n", err)

			return 1
		}

		fmt.Fprintf(stdout, "Dependency installed: %s\n", result.Name)
	}

	return 0
}
