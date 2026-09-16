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
}

// installationInspector reads Node Exporter state without changing the machine.
type installationInspector func(context.Context) (installationState, error)

// freshInstaller performs a complete new Node Exporter installation.
type freshInstallFunc func(context.Context) (dependency.InstallResult, error)

// reconciler decides whether to reuse, install, or reject existing state.
type reconciler struct {
	inspect installationInspector
	install freshInstallFunc
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
		return dependency.InstallResult{
			Name:   "node-exporter",
			Reused: true,
		}, nil
	}

	if state.ServiceExists {
		return dependency.InstallResult{}, fmt.Errorf(
			"existing Node Exporter service is unhealthy; refusing to overwrite it",
		)
	}

	return r.install(ctx)
}
