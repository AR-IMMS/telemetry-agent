package librehardwaremonitor

import (
	"context"
	"net/http"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// installerDependencies groups runtime adapters for one LHM installer.
type installerDependencies struct {
	administrator administratorProbe
	inspect       installationInspector

	stageArchive      archiveStager
	installArchive    archiveInstaller
	writeConfig       configurationWriter
	configureFirewall firewallConfigurator
	registerTask      taskRegistrar
	waitForHealth     healthWaiter

	startupTimeout time.Duration
}

// NewInspector creates the production lifecycle inspector for the
// Agent-owned Libre Hardware Monitor installation.
func NewInspector(
	options Options,
) dependency.Inspector {
	return newDependencyInspector(
		newInstallationInspector(
			options,
			http.DefaultClient,
			defaultHealthPoll,
			runOSProcessOutput,
			currentWindowsUserSID,
		),
	)
}

// newInstaller composes privilege policy, reconciliation, fresh installation,
// and managed-resource updates into the public dependency installer contract.
func newInstaller(
	options Options,
	dependencies installerDependencies,
) dependency.Installer {
	fresh := freshInstaller{
		options:           options,
		startupTimeout:    dependencies.startupTimeout,
		stageArchive:      dependencies.stageArchive,
		installArchive:    dependencies.installArchive,
		writeConfig:       dependencies.writeConfig,
		configureFirewall: dependencies.configureFirewall,
		registerTask:      dependencies.registerTask,
		waitForHealth:     dependencies.waitForHealth,
	}

	reconciler := reconciler{
		inspect:        dependencies.inspect,
		install:        fresh.Install,
		update:         fresh.Update,
	}

	managed := managedInstaller{
		administrator: dependencies.administrator,
		reconcile:     reconciler.Install,
	}

	return managed.Install
}

const (
	defaultStartupTimeout = 30 * time.Second
	defaultHealthPoll     = 500 * time.Millisecond
)

func ownedResources(options Options) []agentstate.OwnedResource {
	return []agentstate.OwnedResource{
		{
			Kind:       "scheduled-task",
			Identifier: options.TaskName,
		},
		{
			Kind:       "firewall-rule",
			Identifier: options.FirewallName,
		},
		{
			Kind:       "directory",
			Identifier: options.InstallDir,
		},
	}
}

// NewInstaller creates the production Windows LHM dependency installer.
func NewInstaller(
	options Options,
	downloader dependency.ArtifactDownloader,
) dependency.Installer {
	artifact := PinnedArtifact()
	runPowerShell := newPowerShellScriptRunner(runOSProcess)

	return newInstaller(
		options,
		installerDependencies{
			administrator: isWindowsAdministrator,
			inspect: newInstallationInspector(
				options,
				http.DefaultClient,
				defaultHealthPoll,
				runOSProcessOutput,
				currentWindowsUserSID,
			),
			stageArchive: func(
				ctx context.Context,
			) (string, func(), error) {
				return StageArchive(ctx, artifact, downloader)
			},
			installArchive: InstallArchive,
			writeConfig:    WriteConfig,
			configureFirewall: func(
				ctx context.Context,
				options Options,
			) error {
				return ConfigureFirewall(ctx, options, runPowerShell)
			},
			registerTask: func(
				ctx context.Context,
				options Options,
			) error {
				userSID, err := currentWindowsUserSID()
				if err != nil {
					return err
				}

				return RegisterTask(
					ctx,
					options,
					userSID,
					runOSProcess,
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
