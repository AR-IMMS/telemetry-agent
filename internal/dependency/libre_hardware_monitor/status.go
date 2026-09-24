package librehardwaremonitor

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// newDependencyInspector maps LHM installation observations into the generic
// dependency lifecycle model.
func newDependencyInspector(
	inspect installationInspector,
) dependency.Inspector {
	return func(ctx context.Context) (dependency.Inspection, error) {
		if inspect == nil {
			return dependency.Inspection{}, fmt.Errorf(
				"Libre Hardware Monitor installation inspector is required",
			)
		}

		state, err := inspect(ctx)
		if err != nil {
			return dependency.Inspection{}, fmt.Errorf(
				"inspect Libre Hardware Monitor installation: %w",
				err,
			)
		}

		if !state.TaskExists || !state.TaskEnabled {
			return dependency.Inspection{}, nil
		}

		return dependency.Inspection{
			Enabled: true,
			Healthy: state.Healthy,
			Drifted: !state.TaskMatches ||
				!state.ConfigMatches ||
				!state.FirewallMatches,
		}, nil
	}
}
