package dependency

import (
	"context"
	"fmt"
	"io"
	"strings"

	runtimeDependencies "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

// Status writes the existing dependency lifecycle presentation and returns its
// established exit code. Runtime inspection and Agent state loading stay with
// the caller.
func Status(
	ctx context.Context,
	stdout, stderr io.Writer,
	listStatuses func(context.Context) ([]runtimeDependencies.StatusResult, error),
	managedNames map[string]struct{},
) int {
	if listStatuses == nil {
		fmt.Fprintln(stderr, "dependency status: lister is not configured")
		return 1
	}

	results, err := listStatuses(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "dependency status: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Dependency status:")
	for _, result := range results {
		if result.InspectionError != nil {
			fmt.Fprintf(stdout, "- %s: unknown (%v)\n", result.Definition.Name, result.InspectionError)
			continue
		}

		name := strings.ToLower(strings.TrimSpace(result.Definition.Name))
		_, managed := managedNames[name]
		if result.Status.Availability == runtimeDependencies.AvailabilityDisabled && !managed {
			fmt.Fprintf(stdout, "- %s: not installed\n", result.Definition.Name)
			continue
		}

		condition := string(result.Status.Health)
		if result.Status.Drifted {
			condition += ", drifted"
		}

		metricsEndpoint := strings.TrimSpace(result.Definition.MetricsEndpoint)
		if metricsEndpoint == "" {
			fmt.Fprintf(stdout, "- %s: %s (%s)\n", result.Definition.Name, result.Status.Availability, condition)
			continue
		}

		fmt.Fprintf(stdout, "- %s: %s (%s) — metrics: %s\n", result.Definition.Name, result.Status.Availability, condition, metricsEndpoint)
	}

	return 0
}
