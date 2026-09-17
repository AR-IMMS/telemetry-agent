package librehardwaremonitor

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// administratorProbe reports whether the current Windows process is elevated.
type administratorProbe func() (bool, error)

// installFunc is the reconciled LHM installation contract.
type installFunc func(context.Context) (dependency.InstallResult, error)

// managedInstaller applies the Administrator policy before reading or changing
// machine-wide LHM Task Scheduler and firewall state.
type managedInstaller struct {
	administrator administratorProbe
	reconcile     installFunc
}

// Install requires Administrator privileges before reconciliation.
func (i managedInstaller) Install(
	ctx context.Context,
) (dependency.InstallResult, error) {
	if err := requireWindowsAdministrator(i.administrator); err != nil {
		return dependency.InstallResult{}, err
	}
	if i.reconcile == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Libre Hardware Monitor reconciler is required",
		)
	}

	return i.reconcile(ctx)
}

// requireWindowsAdministrator rejects execution before any machine-wide action.
func requireWindowsAdministrator(
	probe administratorProbe,
) error {
	if probe == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor Administrator privilege probe is required",
		)
	}

	elevated, err := probe()
	if err != nil {
		return fmt.Errorf(
			"determine Windows Administrator privileges: %w",
			err,
		)
	}
	if !elevated {
		return fmt.Errorf(
			"Libre Hardware Monitor installation requires an Administrator terminal",
		)
	}

	return nil
}
