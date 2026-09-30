//go:build windows

package dependency

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	windowsExporterHealthInspectionTimeout = time.Second
	windowsExporterHealthPollInterval      = 100 * time.Millisecond

	windowsErrorServiceDoesNotExist syscall.Errno = 1060
)

// inspectWindowsExporterInstallation reads the installed service state and
// performs a bounded health check when the service is running.
func inspectWindowsExporterInstallation(
	ctx context.Context,
	healthEndpoint string,
) (windowsExporterInstallationState, error) {
	if ctx == nil {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"Windows Exporter inspection context is required",
		)
	}
	if strings.TrimSpace(healthEndpoint) == "" {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"Windows Exporter health endpoint is required",
		)
	}

	// Connect to the Windows Service Control Manager to inspect the service.
	serviceManager, err := mgr.Connect()
	if err != nil {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"connect to Windows Service Control Manager: %w",
			err,
		)
	}
	defer serviceManager.Disconnect()

	// Open the Windows Exporter service to read its current state.
	service, err := serviceManager.OpenService(windowsExporterServiceName)
	if errors.Is(err, windowsErrorServiceDoesNotExist) {
		return windowsExporterInstallationState{}, nil
	}
	if err != nil {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"open Windows Exporter service: %w",
			err,
		)
	}
	defer service.Close()

	// Read startup configuration before classifying the service lifecycle.
	serviceConfig, err := service.Config()
	if err != nil {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"read Windows Exporter service configuration: %w",
			err,
		)
	}

	// Query the service to determine whether it is currently running.
	status, err := service.Query()
	if err != nil {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"query Windows Exporter service: %w",
			err,
		)
	}

	state, err := windowsExporterInstallationStateFromSCM(
		serviceConfig.StartType,
		status.State,
	)
	if err != nil {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"classify Windows Exporter service startup: %w",
			err,
		)
	}
	if !state.serviceEnabled || !state.serviceRunning {
		return state, nil
	}

	healthContext, cancel := context.WithTimeout(
		ctx,
		windowsExporterHealthInspectionTimeout,
	)
	defer cancel()

	// A failed health check is state information, not an inspection failure.
	// The reconciler will refuse to overwrite this existing unhealthy service.
	state.healthReady = waitForWindowsExporterHealth(
		healthContext,
		http.DefaultClient,
		healthEndpoint,
		windowsExporterHealthPollInterval,
	) == nil

	return state, nil
}

// windowsExporterStartupState maps the SCM startup configuration to the
// Agent lifecycle contract.
func windowsExporterStartupState(
	startType uint32,
) (enabled bool, matches bool, err error) {
	switch startType {
	case mgr.StartAutomatic:
		return true, true, nil

	case mgr.StartManual:
		return true, false, nil

	case mgr.StartDisabled:
		return false, false, nil

	default:
		return false, false, fmt.Errorf(
			"unsupported Windows Exporter service start type %d",
			startType,
		)
	}
}

// windowsExporterInstallationStateFromSCM maps SCM configuration and runtime
// state into the Agent-owned Windows Exporter observation.
func windowsExporterInstallationStateFromSCM(
	startType uint32,
	serviceState svc.State,
) (windowsExporterInstallationState, error) {
	enabled, startupMatches, err := windowsExporterStartupState(startType)
	if err != nil {
		return windowsExporterInstallationState{}, err
	}

	return windowsExporterInstallationState{
		serviceExists:  true,
		serviceEnabled: enabled,
		serviceRunning: serviceState == svc.Running,
		startupMatches: startupMatches,
	}, nil
}
