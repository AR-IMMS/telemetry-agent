package nodeexporter

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// systemdUnitEnabledInspector reports whether one unit starts at boot.
type systemdUnitEnabledInspector func(
	context.Context,
	string,
) (bool, error)

// newDependencyInspector maps Node Exporter's OS-specific state into the
// generic dependency lifecycle inspection contract.
func newDependencyInspector(
	options Options,
	inspect installationInspector,
	inspectEnabled systemdUnitEnabledInspector,
) dependency.Inspector {
	return func(ctx context.Context) (dependency.Inspection, error) {
		if err := options.Validate(); err != nil {
			return dependency.Inspection{}, fmt.Errorf(
				"validate Node Exporter status options: %w",
				err,
			)
		}
		if inspect == nil {
			return dependency.Inspection{}, fmt.Errorf(
				"Node Exporter installation inspector is required",
			)
		}
		if inspectEnabled == nil {
			return dependency.Inspection{}, fmt.Errorf(
				"Node Exporter systemd enabled inspector is required",
			)
		}

		state, err := inspect(ctx)
		if err != nil {
			return dependency.Inspection{}, fmt.Errorf(
				"inspect Node Exporter installation: %w",
				err,
			)
		}
		if !state.ServiceExists {
			return dependency.Inspection{}, nil
		}

		serviceName := filepath.Base(
			strings.TrimSpace(options.ServicePath),
		)

		enabled, err := inspectEnabled(ctx, serviceName)
		if err != nil {
			return dependency.Inspection{}, fmt.Errorf(
				"inspect Node Exporter systemd enabled state: %w",
				err,
			)
		}

		return dependency.Inspection{
			Enabled: enabled,
			Healthy: state.Healthy,
			Drifted: enabled && !state.UnitMatches,
		}, nil
	}
}
