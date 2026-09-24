package librehardwaremonitor

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// installationState describes the current Agent-owned LHM installation.
type installationState struct {
	TaskExists  bool
	Healthy     bool
	TaskEnabled bool

	ConfigMatches   bool
	FirewallMatches bool
	TaskMatches     bool
}

// installationInspector reads LHM state without changing the machine.
type installationInspector func(context.Context) (installationState, error)

// freshInstallFunc performs a complete new LHM installation.
type freshInstallFunc func(context.Context) (dependency.InstallResult, error)

// managedUpdateFunc repairs Agent-owned LHM resource drift without re-download.
type managedUpdateFunc func(context.Context) error

// reconciler chooses reuse, fresh installation, or a safe failure.
type reconciler struct {
	inspect installationInspector
	install freshInstallFunc
	update  managedUpdateFunc
}

// Install reuses only a healthy installation whose managed resources match.
func (r reconciler) Install(
	ctx context.Context,
) (dependency.InstallResult, error) {
	if r.inspect == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Libre Hardware Monitor installation inspector is required",
		)
	}
	if r.install == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Libre Hardware Monitor fresh installer is required",
		)
	}

	state, err := r.inspect(ctx)
	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"inspect Libre Hardware Monitor installation: %w",
			err,
		)
	}

	if !state.TaskExists {
		return r.install(ctx)
	}

	if !state.Healthy {
		return dependency.InstallResult{}, fmt.Errorf(
			"existing Libre Hardware Monitor task is unhealthy; refusing to overwrite it",
		)
	}

	if state.ConfigMatches &&
		state.FirewallMatches &&
		state.TaskMatches {
		return dependency.InstallResult{
			Name:   "libre-hardware-monitor",
			Reused: true,
		}, nil
	}

	if r.update == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Libre Hardware Monitor managed update is required for resource drift",
		)
	}

	if err := r.update(ctx); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"update Libre Hardware Monitor managed resources: %w",
			err,
		)
	}

	return dependency.InstallResult{
		Name:   "libre-hardware-monitor",
		Reused: true,
	}, nil
}
