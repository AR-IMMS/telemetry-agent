//go:build windows

package agentinstallation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func nativeServiceE2EIsAdministrator() (bool, error) {
	administratorsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, fmt.Errorf("create Administrators SID: %w", err)
	}
	elevated, err := windows.Token(0).IsMember(administratorsSID)
	if err != nil {
		return false, fmt.Errorf("check Administrators membership: %w", err)
	}
	return elevated, nil
}

func nativeServiceE2ERequireCleanHost(layout Layout) error {
	for _, path := range []string{layout.InstallDirectory, layout.DataDirectory} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("Agent-owned path already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect Agent-owned path %s: %w", path, err)
		}
	}
	installed, err := windowsServiceExists(layout.ServiceName)
	if err != nil || installed {
		return fmt.Errorf("Agent service is not absent (installed=%t, error=%v)", installed, err)
	}
	collector, supervisor, err := nativeServiceE2EProcessCounts()
	if err != nil || collector != 0 || supervisor != 0 {
		return fmt.Errorf("test host has preexisting runtime processes (collector=%d supervisor=%d error=%v)", collector, supervisor, err)
	}
	return nil
}

func nativeServiceE2ERequireRemoved(layout Layout) error {
	installed, err := windowsServiceExists(layout.ServiceName)
	if err != nil || installed {
		return fmt.Errorf("Agent service remains after uninstall (installed=%t, error=%v)", installed, err)
	}
	for _, path := range []string{layout.InstallDirectory, layout.DataDirectory} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("Agent-owned path remains after uninstall: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect removed Agent path %s: %w", path, err)
		}
	}
	return nil
}

func nativeServiceE2EActive(serviceName string) (bool, error) {
	output, err := exec.Command("sc.exe", "query", serviceName).CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1060 {
			return false, nil
		}
		return false, fmt.Errorf("sc query %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	return regexp.MustCompile(`(?im)^\s*STATE\s*:\s*4\s+RUNNING\s*$`).Match(output), nil
}

func nativeServiceE2ERestart(ctx context.Context, serviceName string) error {
	output, err := exec.CommandContext(ctx, "sc.exe", "stop", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc stop %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	for {
		stopped, queryErr := nativeServiceE2EStopped(serviceName)
		if queryErr == nil && stopped {
			break
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait for service stop: %w", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
	output, err = exec.CommandContext(ctx, "sc.exe", "start", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc start %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func nativeServiceE2EStopped(serviceName string) (bool, error) {
	output, err := exec.Command("sc.exe", "query", serviceName).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("sc query %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	return regexp.MustCompile(`(?im)^\s*STATE\s*:\s*1\s+STOPPED\s*$`).Match(output), nil
}

func nativeServiceE2EProcessCounts() (int, int, error) {
	collector, err := tasklistProcessCount("otelcol-contrib.exe")
	if err != nil {
		return 0, 0, err
	}
	supervisor, err := tasklistProcessCount("opampsupervisor_0.160.0_windows_amd64.exe")
	if err != nil {
		return 0, 0, err
	}
	return collector, supervisor, nil
}

func tasklistProcessCount(imageName string) (int, error) {
	output, err := exec.Command("tasklist.exe", "/FI", "IMAGENAME eq "+imageName, "/FO", "CSV", "/NH").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("tasklist %s: %w: %s", imageName, err, strings.TrimSpace(string(output)))
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.Contains(strings.ToLower(line), strings.ToLower(imageName)) {
			count++
		}
	}
	return count, nil
}

func windowsServiceExists(serviceName string) (bool, error) {
	output, err := exec.Command("sc.exe", "query", serviceName).CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1060 {
		return false, nil
	}
	return false, fmt.Errorf("sc query %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
}
