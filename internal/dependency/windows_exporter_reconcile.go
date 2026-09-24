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
}

// windowsExporterStateInspector reads current service and health state.
type windowsExporterStateInspector func(
	context.Context,
) (windowsExporterInstallationState, error)

// windowsExporterInstallFunc performs a fresh MSI installation when needed.
type windowsExporterInstallFunc func(
	context.Context,
) (InstallResult, error)

// windowsExporterReconciler decides whether an existing installation is safe
// to reuse or whether a fresh installation is required.
type windowsExporterReconciler struct {
	inspect windowsExporterStateInspector
	install windowsExporterInstallFunc
}

// Install reuses a known-healthy service and avoids MSI work in that case.
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
		return InstallResult{
			Name:   windowsExporterDefinition.Name,
			Reused: true,
		}, nil
	}

	// A partially working existing service is deliberately not reinstalled yet.
	// Reconciliation must first learn version and config ownership before it can
	// safely repair or replace a machine-wide Windows service.
	if state.serviceExists {
		return InstallResult{}, fmt.Errorf(
			"Windows Exporter service exists but is not healthy",
		)
	}

	return r.install(ctx)
}
