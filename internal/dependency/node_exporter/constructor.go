package nodeexporter

import (
	"context"
	"net/http"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// installerDependencies groups the runtime adapters used to compose one
// production Node Exporter installer.
type installerDependencies struct {
	effectiveUserID func() int
	inspect         installationInspector

	stageArchive   archiveStager
	installArchive archivePublisher
	writeUnit      systemdUnitWriter
	startService   systemdServiceStarter
	restartService systemdServiceStarter
	waitForHealth  nodeExporterHealthWaiter

	startupTimeout time.Duration
}

// newInstaller composes privilege policy, reconciliation, and fresh install
// behavior into the public dependency installer contract.
func newInstaller(
	options Options,
	dependencies installerDependencies,
) dependency.Installer {
	fresh := freshInstaller{
		options:        options,
		startupTimeout: dependencies.startupTimeout,

		stageArchive:   dependencies.stageArchive,
		installArchive: dependencies.installArchive,
		writeUnit:      dependencies.writeUnit,
		startService:   dependencies.startService,
		restartService: dependencies.restartService,
		waitForHealth:  dependencies.waitForHealth,
	}

	reconciler := reconciler{
		inspect: dependencies.inspect,
		install: fresh.Install,
		update:  fresh.Update,
	}

	managed := managedInstaller{
		effectiveUserID: dependencies.effectiveUserID,
		reconcile:       reconciler.Install,
	}

	return managed.Install
}

const (
	defaultStartupTimeout = 30 * time.Second
	defaultHealthPoll     = 500 * time.Millisecond
)

// NewInstaller creates the production Linux Node Exporter dependency installer.
func NewInstaller(
	options Options,
	downloader dependency.ArtifactDownloader,
) dependency.Installer {
	artifact := PinnedArtifact()
	runSystemd := newOSSystemdRunner()

	return newInstaller(
		options,
		installerDependencies{
			effectiveUserID: currentEffectiveUserID,
			inspect: newInstallationInspector(
				options,
				http.DefaultClient,
				defaultHealthPoll,
			),
			stageArchive: func(
				ctx context.Context,
			) (string, func(), error) {
				return StageArchive(ctx, artifact, downloader)
			},
			installArchive: func(archivePath string) error {
				_, err := InstallArchive(
					archivePath,
					artifact,
					options.InstallDir,
				)

				return err
			},
			writeUnit: WriteSystemdUnit,
			startService: func(
				ctx context.Context,
				serviceName string,
			) error {
				return EnableAndStartSystemdService(
					ctx,
					serviceName,
					runSystemd,
				)
			},
			restartService: func(
				ctx context.Context,
				serviceName string,
			) error {
				return ReloadAndRestartSystemdService(
					ctx,
					serviceName,
					runSystemd,
				)
			},
			waitForHealth: func(
				ctx context.Context,
				endpoint string,
			) error {
				return WaitForHealth(
					ctx,
					http.DefaultClient,
					endpoint,
					defaultHealthPoll,
				)
			},
			startupTimeout: defaultStartupTimeout,
		},
	)
}
