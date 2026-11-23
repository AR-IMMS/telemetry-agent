package dependency

import (
	"context"
	"fmt"
	"io"

	runtimeDependencies "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

// List writes the existing dependency catalog presentation and returns its
// established exit code. The callback owns catalog lookup and Runtime errors.
func List(
	ctx context.Context,
	stdout, stderr io.Writer,
	listDefinitions func(context.Context) ([]runtimeDependencies.Definition, error),
) int {
	if listDefinitions == nil {
		fmt.Fprintln(stderr, "list dependencies: lister is not configured")
		return 1
	}

	definitions, err := listDefinitions(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "list dependencies: %v\n", err)
		return 1
	}
	if len(definitions) == 0 {
		fmt.Fprintln(stdout, "No dependencies are available for this operating system.")
		return 0
	}

	fmt.Fprintln(stdout, "Available dependencies:")
	for _, definition := range definitions {
		fmt.Fprintf(stdout, "- %s: %s — %s\n", definition.Name, definition.DisplayName, definition.Description)
	}
	return 0
}
