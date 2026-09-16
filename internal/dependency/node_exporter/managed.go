package nodeexporter

import (
	"context"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// managedInstaller applies root policy before reconciliation or installation.
type managedInstaller struct {
	effectiveUserID func() int
	reconcile       freshInstallFunc
}

// Install checks root privileges before reading or changing Node Exporter state.
func (i managedInstaller) Install(
	ctx context.Context,
) (dependency.InstallResult, error) {
	if err := requireLinuxRoot(i.effectiveUserID); err != nil {
		return dependency.InstallResult{}, err
	}

	return i.reconcile(ctx)
}
