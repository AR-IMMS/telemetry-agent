package dependency

import (
	"context"
	"net/http"
	"time"
)

const (
	defaultWindowsExporterStartupTimeout = 30 * time.Second
	defaultWindowsExporterHealthPoll     = 500 * time.Millisecond
)

// windowsExporterReconcileFunc runs the read-only reconciliation decision.
type windowsExporterReconcileFunc func(
	context.Context,
) (InstallResult, error)

// managedWindowsExporterInstaller applies privilege policy before reading or
// changing machine-wide Windows Exporter state.
type managedWindowsExporterInstaller struct {
	administrator administratorProbe
	reconcile     windowsExporterReconcileFunc
}

// Install checks Administrator privileges before reconciliation or installation.
func (i managedWindowsExporterInstaller) Install(
	ctx context.Context,
) (InstallResult, error) {
	if err := requireWindowsAdministrator(i.administrator); err != nil {
		return InstallResult{}, err
	}

	return i.reconcile(ctx)
}

// NewWindowsExporterInstaller composes the live Windows service, network, and
// process adapters used by the dependency CLI.
func NewWindowsExporterInstaller(
	options WindowsExporterOptions,
	downloader ArtifactDownloader,
) WindowsExporterInstaller {
	freshInstaller := windowsExporterInstaller{
		options:        options,
		artifact:       PinnedWindowsExporterArtifact(),
		downloader:     downloader,
		administrator:  isWindowsAdministrator,
		runProcess:     runOSProcess,
		startupTimeout: defaultWindowsExporterStartupTimeout,
		waitForHealth: func(
			ctx context.Context,
			endpoint string,
		) error {
			return waitForWindowsExporterHealth(
				ctx,
				http.DefaultClient,
				endpoint,
				defaultWindowsExporterHealthPoll,
			)
		},
	}

	reconciler := windowsExporterReconciler{
		inspect: func(
			ctx context.Context,
		) (windowsExporterInstallationState, error) {
			return inspectWindowsExporterInstallation(
				ctx,
				windowsExporterHealthEndpoint(options.ListenAddress),
			)
		},
		install: freshInstaller.Install,
	}

	managedInstaller := managedWindowsExporterInstaller{
		administrator: isWindowsAdministrator,
		reconcile:     reconciler.Install,
	}

	return managedInstaller.Install
}
