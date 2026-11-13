//go:build windows

package agentinstallation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	windowsServiceTransitionTimeout = 30 * time.Second
	windowsServicePollInterval      = 100 * time.Millisecond
	windowsServiceDescription       = "AR-IMMS Telemetry Agent"
	windowsServiceRecoveryReset     = 24 * 60 * 60
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
		return configureWindowsServiceRecovery(service)
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
	return configureWindowsServiceRecovery(service)
}

func configureWindowsServiceRecovery(service *mgr.Service) error {
	if err := service.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, windowsServiceRecoveryReset); err != nil {
		return fmt.Errorf("configure bounded Agent service recovery actions: %w", err)
	}
	if err := service.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("enable Agent service recovery after reported failures: %w", err)
	}
	return nil
}

func (windowsSCMServiceManager) Stop(
	ctx context.Context,
	serviceName string,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows service context is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("Windows service name is required")
	}
	transitionContext, cancel := context.WithTimeout(ctx, windowsServiceTransitionTimeout)
	defer cancel()
	serviceManager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows Service Control Manager: %w", err)
	}
	defer serviceManager.Disconnect()
	service, err := serviceManager.OpenService(serviceName)
	if errors.Is(err, windowsErrorServiceDoesNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open Windows service %q: %w", serviceName, err)
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("query Windows service %q: %w", serviceName, err)
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State == svc.StopPending {
		if err := waitForWindowsServiceState(transitionContext, service, svc.Stopped); err != nil {
			return fmt.Errorf("wait for Windows service %q to stop: %w", serviceName, err)
		}
		return nil
	}
	if status.State == svc.StartPending {
		if err := waitForWindowsServiceState(transitionContext, service, svc.Running); err != nil {
			return fmt.Errorf("wait for Windows service %q startup before stopping: %w", serviceName, err)
		}
	}
	if _, err := service.Control(svc.Stop); err != nil {
		return fmt.Errorf("stop Windows service %q: %w", serviceName, err)
	}
	if err := waitForWindowsServiceState(transitionContext, service, svc.Stopped); err != nil {
		return fmt.Errorf("wait for Windows service %q to stop: %w", serviceName, err)
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
	var processHandle windows.Handle
	if status.State != svc.Stopped && status.ProcessId != 0 {
		processHandle, err = openWindowsServiceProcess(status.ProcessId)
		if err != nil {
			return fmt.Errorf(
				"open Windows service %q process %d: %w",
				serviceName,
				status.ProcessId,
				err,
			)
		}
		if processHandle != 0 {
			defer windows.CloseHandle(processHandle)
		}
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
	if processHandle != 0 {
		if err := waitForWindowsServiceProcessHandleExit(
			transitionContext,
			processHandle,
		); err != nil {
			return fmt.Errorf(
				"wait for Windows service %q process to exit: %w",
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
