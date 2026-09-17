package nodeexporter

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// systemdServiceInspector reports whether the named systemd service exists.
type systemdServiceInspector func(
	context.Context,
	string,
) (bool, error)

// healthInspector checks whether the local Node Exporter endpoint is healthy.
type healthInspector func(context.Context) error

// systemdUnitMatcher reports whether the Agent-owned unit matches the current
// expected Node Exporter unit.
type systemdUnitMatcher func(context.Context) (bool, error)

// inspectNodeExporterInstallation reads service, health, and managed-unit state
// without changing the machine.
func inspectNodeExporterInstallation(
	ctx context.Context,
	serviceName string,
	inspectService systemdServiceInspector,
	checkHealth healthInspector,
	matchUnit systemdUnitMatcher,
) (installationState, error) {
	serviceName = strings.TrimSpace(serviceName)

	if serviceName == "" {
		return installationState{}, fmt.Errorf(
			"Node Exporter systemd service name is required",
		)
	}
	if inspectService == nil {
		return installationState{}, fmt.Errorf(
			"Node Exporter systemd service inspector is required",
		)
	}
	if checkHealth == nil {
		return installationState{}, fmt.Errorf(
			"Node Exporter health inspector is required",
		)
	}
	if matchUnit == nil {
		return installationState{}, fmt.Errorf(
			"Node Exporter systemd unit matcher is required",
		)
	}

	exists, err := inspectService(ctx, serviceName)
	if err != nil {
		return installationState{}, fmt.Errorf(
			"inspect Node Exporter systemd service: %w",
			err,
		)
	}
	if !exists {
		return installationState{}, nil
	}

	if err := checkHealth(ctx); err != nil {
		return installationState{
			ServiceExists: true,
		}, nil
	}

	matches, err := matchUnit(ctx)
	if err != nil {
		return installationState{}, fmt.Errorf(
			"match Node Exporter systemd unit: %w",
			err,
		)
	}

	return installationState{
		ServiceExists: true,
		Healthy:       true,
		UnitMatches:   matches,
	}, nil
}

// newInstallationInspector composes the Agent-owned unit-file check with the
// local Node Exporter metrics health check.
func newInstallationInspector(
	options Options,
	client *http.Client,
	healthPollInterval time.Duration,
) installationInspector {
	return func(ctx context.Context) (installationState, error) {
		if err := options.Validate(); err != nil {
			return installationState{}, fmt.Errorf(
				"validate Node Exporter installation options: %w",
				err,
			)
		}

		serviceName := filepath.Base(
			strings.TrimSpace(options.ServicePath),
		)
		healthEndpoint := "http://" +
			strings.TrimSpace(options.ListenAddress) +
			"/metrics"

		return inspectNodeExporterInstallation(
			ctx,
			serviceName,
			func(
				ctx context.Context,
				serviceName string,
			) (bool, error) {
				return systemdUnitExists(options.ServicePath)
			},
			func(ctx context.Context) error {
				return WaitForHealth(
					ctx,
					client,
					healthEndpoint,
					healthPollInterval,
				)
			},
			func(ctx context.Context) (bool, error) {
				return systemdUnitMatches(options)
			},
		)
	}
}
