package nodeexporter

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// installationState describes the current Agent-owned Node Exporter service.
type installationState struct {
	ServiceExists bool
	Healthy       bool

	// UnitMatches reports whether the Agent-owned systemd unit matches the
	// version currently rendered by this Agent release.
	UnitMatches bool
}

// installationInspector reads Node Exporter state without changing the machine.
type installationInspector func(context.Context) (installationState, error)

// freshInstaller performs a complete new Node Exporter installation.
type freshInstallFunc func(context.Context) (dependency.InstallResult, error)

// managedUnitUpdateFunc replaces a stale Agent-owned unit and confirms the
// restarted service becomes healthy.
type managedUnitUpdateFunc func(
	context.Context,
) (dependency.InstallResult, error)

// reconciler decides whether to reuse, install, or reject existing state.
type reconciler struct {
	inspect installationInspector
	install freshInstallFunc
	update  managedUnitUpdateFunc
}

// Install reuses a healthy service and only installs when no service exists.
func (r reconciler) Install(
	ctx context.Context,
) (dependency.InstallResult, error) {
	if r.inspect == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Node Exporter installation inspector is required",
		)
	}
	if r.install == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Node Exporter fresh installer is required",
		)
	}

	state, err := r.inspect(ctx)
	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"inspect Node Exporter installation: %w",
			err,
		)
	}

	if state.ServiceExists && state.Healthy {
		if state.UnitMatches {
			return dependency.InstallResult{
				Name:   "node-exporter",
				Reused: true,
			}, nil
		}

		if r.update == nil {
			return dependency.InstallResult{}, fmt.Errorf(
				"Node Exporter managed unit updater is required",
			)
		}

		return r.update(ctx)
	}

	if state.ServiceExists {
		return dependency.InstallResult{}, fmt.Errorf(
			"existing Node Exporter service is unhealthy; refusing to overwrite it",
		)
	}

	return r.install(ctx)
}
