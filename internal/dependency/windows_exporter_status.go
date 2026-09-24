package dependency

import (
	"context"
	"fmt"
)

type windowsExporterConfigInspector func(
	WindowsExporterOptions,
) (bool, error)

// newWindowsExporterDependencyInspector maps Windows Exporter observations
// into the generic dependency lifecycle model.
func newWindowsExporterDependencyInspector(
	options WindowsExporterOptions,
	inspect windowsExporterStateInspector,
	inspectConfig windowsExporterConfigInspector,
) Inspector {
	return func(ctx context.Context) (Inspection, error) {
		if err := options.Validate(); err != nil {
			return Inspection{}, fmt.Errorf(
				"validate Windows Exporter inspection options: %w",
				err,
			)
		}
		if inspect == nil {
			return Inspection{}, fmt.Errorf(
				"Windows Exporter state inspector is required",
			)
		}
		if inspectConfig == nil {
			return Inspection{}, fmt.Errorf(
				"Windows Exporter configuration inspector is required",
			)
		}

		state, err := inspect(ctx)
		if err != nil {
			return Inspection{}, fmt.Errorf(
				"inspect Windows Exporter installation: %w",
				err,
			)
		}

		if !state.serviceExists || !state.serviceEnabled {
			return Inspection{}, nil
		}

		configMatches, err := inspectConfig(options)
		if err != nil {
			return Inspection{}, fmt.Errorf(
				"inspect Windows Exporter configuration: %w",
				err,
			)
		}

		return Inspection{
			Enabled: true,
			Healthy: state.serviceRunning && state.healthReady,
			Drifted: !state.startupMatches || !configMatches,
		}, nil
	}
}

// NewWindowsExporterInspector creates the production lifecycle inspector for
// the Agent-owned Windows Exporter service.
func NewWindowsExporterInspector(
	options WindowsExporterOptions,
) Inspector {
	return newWindowsExporterDependencyInspector(
		options,
		func(
			ctx context.Context,
		) (windowsExporterInstallationState, error) {
			return inspectWindowsExporterInstallation(
				ctx,
				windowsExporterHealthEndpoint(options.ListenAddress),
			)
		},
		WindowsExporterConfigMatches,
	)
}
