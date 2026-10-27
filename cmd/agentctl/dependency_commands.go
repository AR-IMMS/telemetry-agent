package main

import (
	"context"
	"fmt"
	"io"
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
