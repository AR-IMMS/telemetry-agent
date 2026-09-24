package dependency

import (
	"context"
	"fmt"
)

// windowsExporterInstallationState is the observed local service state.
type windowsExporterInstallationState struct {
	serviceExists  bool
	serviceEnabled bool
	serviceRunning bool
	healthReady    bool
	startupMatches bool
	configMatches  bool
}

// windowsExporterStateInspector reads current service and health state.
type windowsExporterStateInspector func(
	context.Context,
) (windowsExporterInstallationState, error)

// windowsExporterInstallFunc performs a fresh installation or reconfigures a
// healthy Windows Exporter installation with Agent-owned settings.
type windowsExporterInstallFunc func(
	context.Context,
) (InstallResult, error)

// windowsExporterReconciler decides whether an existing installation is safe
// to reuse or whether Agent-owned configuration must be applied.
type windowsExporterReconciler struct {
	inspect windowsExporterStateInspector
	install windowsExporterInstallFunc
}

// Install reuses a healthy matching service, reconfigures healthy drift, and
// refuses to overwrite an unhealthy existing service.
func (r windowsExporterReconciler) Install(
	ctx context.Context,
) (InstallResult, error) {
	if ctx == nil {
		return InstallResult{}, fmt.Errorf(
			"Windows Exporter reconciliation context is required",
		)
	}
	if r.inspect == nil {
		return InstallResult{}, fmt.Errorf(
			"Windows Exporter state inspector is required",
		)
	}
	if r.install == nil {
		return InstallResult{}, fmt.Errorf(
			"Windows Exporter fresh installer is required",
		)
	}

	state, err := r.inspect(ctx)
	if err != nil {
		return InstallResult{}, fmt.Errorf(
			"inspect Windows Exporter installation: %w",
			err,
		)
	}

	if state.serviceExists &&
		state.serviceRunning &&
		state.healthReady {
		if state.startupMatches && state.configMatches {
			return InstallResult{
				Name:   windowsExporterDefinition.Name,
				Reused: true,
			}, nil
		}

		return r.install(ctx)
	}

	if state.serviceExists {
		if !state.configMatches {
			return r.install(ctx)
		}

		return InstallResult{}, fmt.Errorf(
			"Windows Exporter service exists but is not healthy",
		)
	}

	return r.install(ctx)
}
