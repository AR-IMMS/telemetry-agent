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
	windowsExporterServiceName = "windows_exporter"

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

	// Query the service to determine whether it is running and healthy.
	status, err := service.Query()
	if err != nil {
		return windowsExporterInstallationState{}, fmt.Errorf(
			"query Windows Exporter service: %w",
			err,
		)
	}

	// The service exists, so we can report its running state and health.
	state := windowsExporterInstallationState{
		serviceExists:  true,
		serviceRunning: status.State == svc.Running,
	}
	if !state.serviceRunning {
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
