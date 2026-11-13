//go:build windows

package agentinstallation

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

func waitForWindowsServiceProcessExit(ctx context.Context, processID uint32) error {
	if ctx == nil {
		return fmt.Errorf("Windows service process wait context is required")
	}
	processHandle, err := openWindowsServiceProcess(processID)
	if err != nil {
		return fmt.Errorf("open Windows service process %d: %w", processID, err)
	}
	if processHandle == 0 {
		return nil
	}
	defer windows.CloseHandle(processHandle)
	return waitForWindowsServiceProcessHandleExit(ctx, processHandle)
}

func openWindowsServiceProcess(processID uint32) (windows.Handle, error) {
	if processID == 0 {
		return 0, nil
	}
	processHandle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, processID)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return processHandle, nil
}

func waitForWindowsServiceProcessHandleExit(
	ctx context.Context,
	processHandle windows.Handle,
) error {
	if ctx == nil {
		return fmt.Errorf("Windows service process wait context is required")
	}
	for {
		result, err := windows.WaitForSingleObject(
			processHandle,
			uint32(windowsServicePollInterval.Milliseconds()),
		)
		if err != nil {
			return fmt.Errorf("wait for Windows service process handle: %w", err)
		}
		if result == windows.WAIT_OBJECT_0 {
			return nil
		}
		if result != uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("wait for Windows service process returned %d", result)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
