//go:build windows

package agentinstallation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	windowsServiceTransitionTimeout = 30 * time.Second
	windowsServicePollInterval      = 100 * time.Millisecond
	windowsServiceDescription       = "AR-IMMS Telemetry Agent"
)

var windowsErrorServiceDoesNotExist syscall.Errno = 1060

type windowsSCMServiceManager struct{}

func newWindowsSCMServiceManager() windowsServiceManager {
	return windowsSCMServiceManager{}
}

func (windowsSCMServiceManager) Ensure(
	ctx context.Context,
	definition windowsServiceDefinition,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows service context is required")
	}
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("Windows service name is required")
	}
	if strings.TrimSpace(definition.ExecutablePath) == "" {
		return fmt.Errorf("Windows service executable path is required")
	}

	serviceManager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf(
			"connect to Windows Service Control Manager: %w",
			err,
		)
	}
	defer serviceManager.Disconnect()

	service, err := serviceManager.OpenService(definition.Name)
	if errors.Is(err, windowsErrorServiceDoesNotExist) {
		service, err = serviceManager.CreateService(
			definition.Name,
			definition.ExecutablePath,
			mgr.Config{
				StartType:   mgr.StartAutomatic,
				DisplayName: definition.DisplayName,
				Description: windowsServiceDescription,
			},
			definition.Args...,
		)
		if err != nil {
			return fmt.Errorf(
				"create Windows service %q: %w",
				definition.Name,
				err,
			)
		}
		defer service.Close()

		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"open Windows service %q: %w",
			definition.Name,
			err,
		)
	}
	defer service.Close()

	config, err := service.Config()
	if err != nil {
		return fmt.Errorf(
			"read Windows service %q configuration: %w",
			definition.Name,
			err,
		)
	}

	config.StartType = mgr.StartAutomatic
	config.BinaryPathName = definition.commandLine()
	config.DisplayName = definition.DisplayName
	config.Description = windowsServiceDescription

	if err := service.UpdateConfig(config); err != nil {
		return fmt.Errorf(
			"update Windows service %q configuration: %w",
			definition.Name,
			err,
		)
	}

	return nil
}

func (windowsSCMServiceManager) Restart(
	ctx context.Context,
	serviceName string,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows service context is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("Windows service name is required")
	}

	transitionContext, cancel := context.WithTimeout(
		ctx,
		windowsServiceTransitionTimeout,
	)
	defer cancel()

	serviceManager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf(
			"connect to Windows Service Control Manager: %w",
			err,
		)
	}
	defer serviceManager.Disconnect()

	service, err := serviceManager.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf(
			"open Windows service %q: %w",
			serviceName,
			err,
		)
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return fmt.Errorf(
			"query Windows service %q: %w",
			serviceName,
			err,
		)
	}

	if status.State != svc.Stopped {
		if _, err := service.Control(svc.Stop); err != nil {
			return fmt.Errorf(
				"stop Windows service %q: %w",
				serviceName,
				err,
			)
		}

		if err := waitForWindowsServiceState(
			transitionContext,
			service,
			svc.Stopped,
		); err != nil {
			return fmt.Errorf(
				"wait for Windows service %q to stop: %w",
				serviceName,
				err,
			)
		}
	}

	if err := service.Start(); err != nil {
		return fmt.Errorf(
			"start Windows service %q: %w",
			serviceName,
			err,
		)
	}

	if err := waitForWindowsServiceState(
		transitionContext,
		service,
		svc.Running,
	); err != nil {
		return fmt.Errorf(
			"wait for Windows service %q to start: %w",
			serviceName,
			err,
		)
	}

	return nil
}

func waitForWindowsServiceState(
	ctx context.Context,
	service *mgr.Service,
	want svc.State,
) error {
	ticker := time.NewTicker(windowsServicePollInterval)
	defer ticker.Stop()

	for {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == want {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
		}
	}
}

func (windowsSCMServiceManager) Remove(
	ctx context.Context,
	serviceName string,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows service context is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("Windows service name is required")
	}

	transitionContext, cancel := context.WithTimeout(
		ctx,
		windowsServiceTransitionTimeout,
	)
	defer cancel()

	serviceManager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf(
			"connect to Windows Service Control Manager: %w",
			err,
		)
	}
	defer serviceManager.Disconnect()

	service, err := serviceManager.OpenService(serviceName)
	if errors.Is(err, windowsErrorServiceDoesNotExist) {
		// The desired final state is already reached.
		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"open Windows service %q: %w",
			serviceName,
			err,
		)
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return fmt.Errorf(
			"query Windows service %q: %w",
			serviceName,
			err,
		)
	}

	if status.State != svc.Stopped {
		if _, err := service.Control(svc.Stop); err != nil {
			return fmt.Errorf(
				"stop Windows service %q: %w",
				serviceName,
				err,
			)
		}

		if err := waitForWindowsServiceState(
			transitionContext,
			service,
			svc.Stopped,
		); err != nil {
			return fmt.Errorf(
				"wait for Windows service %q to stop: %w",
				serviceName,
				err,
			)
		}
	}

	if err := service.Delete(); err != nil {
		return fmt.Errorf(
			"delete Windows service %q: %w",
			serviceName,
			err,
		)
	}

	return nil
}
