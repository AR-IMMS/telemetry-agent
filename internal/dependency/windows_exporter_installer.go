package dependency

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// windowsExporterHealthWaiter waits until the local exporter is ready.
type windowsExporterHealthWaiter func(context.Context, string) error

// windowsExporterInstaller coordinates one fresh Windows Exporter installation.
// It is kept injectable so unit tests never invoke msiexec.
type windowsExporterInstaller struct {
	options       WindowsExporterOptions
	artifact      WindowsExporterArtifact
	downloader    ArtifactDownloader
	administrator administratorProbe
	runProcess    func(context.Context, processCommand) error

	// startupTimeout bounds only the post-MSI readiness phase.
	startupTimeout time.Duration

	// waitForHealth confirms that the Windows Service actually became usable.
	waitForHealth windowsExporterHealthWaiter
}

// Install performs the safe installation sequence for one verified MSI.
func (i windowsExporterInstaller) Install(
	ctx context.Context,
) (InstallResult, error) {
	if ctx == nil {
		return InstallResult{}, fmt.Errorf("Windows Exporter installation context is required")
	}

	if err := i.validate(); err != nil {
		return InstallResult{}, err
	}

	// Privilege validation happens before config writes, downloads, or MSI work.
	if err := requireWindowsAdministrator(i.administrator); err != nil {
		return InstallResult{}, err
	}

	if err := WriteWindowsExporterConfig(i.options); err != nil {
		return InstallResult{}, err
	}

	msiPath, cleanup, err := stageWindowsExporterMSI(
		ctx,
		i.artifact,
		i.downloader,
	)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	command, err := buildWindowsExporterMSIInstallCommand(
		msiPath,
		i.options,
	)
	if err != nil {
		return InstallResult{}, err
	}

	if err := i.runProcess(ctx, command); err != nil {
		return InstallResult{}, fmt.Errorf(
			"run Windows Exporter MSI installer: %w",
			err,
		)
	}

	healthContext, cancelHealth := context.WithTimeout(
		ctx,
		i.startupTimeout,
	)
	defer cancelHealth()

	if err := i.waitForHealth(
		healthContext,
		windowsExporterHealthEndpoint(i.options.ListenAddress),
	); err != nil {
		return InstallResult{}, fmt.Errorf(
			"wait for Windows Exporter health: %w",
			err,
		)
	}

	return InstallResult{
		Name: windowsExporterDefinition.Name,
		OwnedResources: []agentstate.OwnedResource{
			{
				Kind:       "windows-service",
				Identifier: windowsExporterServiceName,
			},
			{
				Kind:       "msi-product",
				Identifier: "windows_exporter",
			},
			{
				Kind:       "config-file",
				Identifier: i.options.ConfigPath,
			},
		},
	}, nil
}

// validate rejects missing collaborators before the installer changes disk state.
func (i windowsExporterInstaller) validate() error {
	if err := i.options.Validate(); err != nil {
		return err
	}
	if i.downloader == nil {
		return fmt.Errorf("Windows Exporter artifact downloader is required")
	}
	if i.administrator == nil {
		return fmt.Errorf("Windows Administrator privilege probe is required")
	}
	if i.runProcess == nil {
		return fmt.Errorf("Windows Exporter process runner is required")
	}
	if i.startupTimeout <= 0 {
		return fmt.Errorf("Windows Exporter startup timeout must be positive")
	}
	if i.waitForHealth == nil {
		return fmt.Errorf("Windows Exporter health waiter is required")
	}

	return nil
}

// windowsExporterHealthEndpoint derives the local health URL from the
// configured loopback listener.
func windowsExporterHealthEndpoint(
	listenAddress string,
) string {
	return "http://" + strings.TrimSpace(listenAddress) + "/health"
}
