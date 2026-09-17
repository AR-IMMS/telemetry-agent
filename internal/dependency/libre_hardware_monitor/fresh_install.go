package librehardwaremonitor

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// archiveStager creates a verified archive and returns its cleanup function.
type archiveStager func(context.Context) (string, func(), error)

// archiveInstaller publishes the verified LHM archive into InstallDir.
type archiveInstaller func(string, string) error

// configurationWriter persists the Agent-owned LHM configuration.
type configurationWriter func(Options) error

// firewallConfigurator applies the managed inbound-block firewall rule.
type firewallConfigurator func(context.Context, Options) error

// taskRegistrar replaces and starts the managed Scheduled Task.
type taskRegistrar func(context.Context, Options) error

// healthWaiter waits for the local LHM metrics endpoint.
type healthWaiter func(context.Context, string) error

// freshInstaller performs installation only after reconciliation finds no LHM.
type freshInstaller struct {
	options        Options
	startupTimeout time.Duration

	stageArchive      archiveStager
	installArchive    archiveInstaller
	writeConfig       configurationWriter
	configureFirewall firewallConfigurator
	registerTask      taskRegistrar
	waitForHealth     healthWaiter
}

// Install stages, publishes, configures, starts, and verifies LHM.
func (i freshInstaller) Install(
	ctx context.Context,
) (dependency.InstallResult, error) {
	if err := i.options.Validate(); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"validate Libre Hardware Monitor installation options: %w",
			err,
		)
	}
	if i.startupTimeout <= 0 {
		return dependency.InstallResult{}, fmt.Errorf(
			"Libre Hardware Monitor startup timeout must be positive",
		)
	}
	if i.stageArchive == nil ||
		i.installArchive == nil ||
		i.writeConfig == nil ||
		i.configureFirewall == nil ||
		i.registerTask == nil ||
		i.waitForHealth == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Libre Hardware Monitor fresh installer is incomplete",
		)
	}

	archivePath, cleanup, err := i.stageArchive(ctx)
	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"stage Libre Hardware Monitor archive: %w",
			err,
		)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if err := i.installArchive(archivePath, i.options.InstallDir); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"install Libre Hardware Monitor archive: %w",
			err,
		)
	}

	if err := i.writeConfig(i.options); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"write Libre Hardware Monitor configuration: %w",
			err,
		)
	}

	if err := i.configureFirewall(ctx, i.options); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"configure Libre Hardware Monitor firewall: %w",
			err,
		)
	}

	if err := i.registerTask(ctx, i.options); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"register Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	startupContext, cancel := context.WithTimeout(ctx, i.startupTimeout)
	defer cancel()

	healthEndpoint := "http://127.0.0.1:" +
		strconv.Itoa(i.options.ListenPort) +
		"/metrics"

	if err := i.waitForHealth(startupContext, healthEndpoint); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"wait for Libre Hardware Monitor health: %w",
			err,
		)
	}

	return dependency.InstallResult{
		Name: "libre-hardware-monitor",
	}, nil
}

// Update repairs managed config, firewall, and task drift without downloading
// or replacing the existing LHM installation directory.
func (i freshInstaller) Update(
	ctx context.Context,
) error {
	if err := i.options.Validate(); err != nil {
		return fmt.Errorf(
			"validate Libre Hardware Monitor update options: %w",
			err,
		)
	}
	if i.startupTimeout <= 0 {
		return fmt.Errorf(
			"Libre Hardware Monitor startup timeout must be positive",
		)
	}
	if i.writeConfig == nil ||
		i.configureFirewall == nil ||
		i.registerTask == nil ||
		i.waitForHealth == nil {
		return fmt.Errorf(
			"Libre Hardware Monitor managed updater is incomplete",
		)
	}

	if err := i.writeConfig(i.options); err != nil {
		return fmt.Errorf(
			"write Libre Hardware Monitor configuration: %w",
			err,
		)
	}

	if err := i.configureFirewall(ctx, i.options); err != nil {
		return fmt.Errorf(
			"configure Libre Hardware Monitor firewall: %w",
			err,
		)
	}

	if err := i.registerTask(ctx, i.options); err != nil {
		return fmt.Errorf(
			"register Libre Hardware Monitor scheduled task: %w",
			err,
		)
	}

	startupContext, cancel := context.WithTimeout(ctx, i.startupTimeout)
	defer cancel()

	healthEndpoint := "http://127.0.0.1:" +
		strconv.Itoa(i.options.ListenPort) +
		"/metrics"

	if err := i.waitForHealth(startupContext, healthEndpoint); err != nil {
		return fmt.Errorf(
			"wait for Libre Hardware Monitor health after update: %w",
			err,
		)
	}

	return nil
}
