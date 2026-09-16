package nodeexporter

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// archiveStager prepares a verified archive and returns its cleanup function.
type archiveStager func(context.Context) (string, func(), error)

// archivePublisher atomically publishes the Node Exporter binary.
type archivePublisher func(string) error

// systemdUnitWriter persists the Agent-owned systemd unit.
type systemdUnitWriter func(Options) error

// systemdServiceStarter reloads, enables, and starts the service.
type systemdServiceStarter func(context.Context, string) error

// nodeExporterHealthWaiter waits for the local metrics endpoint.
type nodeExporterHealthWaiter func(context.Context, string) error

// freshInstaller performs installation only after reconciliation found no unit.
type freshInstaller struct {
	options        Options
	startupTimeout time.Duration

	stageArchive   archiveStager
	installArchive archivePublisher
	writeUnit      systemdUnitWriter
	startService   systemdServiceStarter
	waitForHealth  nodeExporterHealthWaiter
}

// Install stages, publishes, starts, and verifies one Node Exporter instance.
func (i freshInstaller) Install(
	ctx context.Context,
) (dependency.InstallResult, error) {
	if err := i.options.Validate(); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"validate Node Exporter installation options: %w",
			err,
		)
	}
	if i.startupTimeout <= 0 {
		return dependency.InstallResult{}, fmt.Errorf(
			"Node Exporter startup timeout must be positive",
		)
	}
	if i.stageArchive == nil ||
		i.installArchive == nil ||
		i.writeUnit == nil ||
		i.startService == nil ||
		i.waitForHealth == nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"Node Exporter fresh installer is incomplete",
		)
	}

	archivePath, cleanup, err := i.stageArchive(ctx)
	if err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"stage Node Exporter archive: %w",
			err,
		)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if err := i.installArchive(archivePath); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"install Node Exporter archive: %w",
			err,
		)
	}

	if err := i.writeUnit(i.options); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"write Node Exporter systemd unit: %w",
			err,
		)
	}

	serviceName := filepath.Base(
		strings.TrimSpace(i.options.ServicePath),
	)

	if err := i.startService(ctx, serviceName); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"start Node Exporter systemd service: %w",
			err,
		)
	}

	startupContext, cancel := context.WithTimeout(ctx, i.startupTimeout)
	defer cancel()

	healthEndpoint := "http://" +
		strings.TrimSpace(i.options.ListenAddress) +
		"/metrics"

	if err := i.waitForHealth(startupContext, healthEndpoint); err != nil {
		return dependency.InstallResult{}, fmt.Errorf(
			"wait for Node Exporter health: %w",
			err,
		)
	}

	return dependency.InstallResult{
		Name: "node-exporter",
	}, nil
}
