//go:build linux

package agentinstallation

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func nativeServiceE2EIsAdministrator() (bool, error) {
	return os.Geteuid() == 0, nil
}

func nativeServiceE2ERequireCleanHost(layout Layout) error {
	for _, path := range []string{layout.InstallDirectory, layout.DataDirectory} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("Agent-owned path already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect Agent-owned path %s: %w", path, err)
		}
	}
	state, err := systemdLoadState(layout.ServiceName)
	if err != nil || state != "not-found" {
		return fmt.Errorf("Agent service is not absent (load_state=%q, error=%v)", state, err)
	}
	collector, supervisor, err := nativeServiceE2EProcessCounts()
	if err != nil || collector != 0 || supervisor != 0 {
		return fmt.Errorf("test host has preexisting runtime processes (collector=%d supervisor=%d error=%v)", collector, supervisor, err)
	}
	return nil
}

func nativeServiceE2ERequireRemoved(layout Layout) error {
	if state, err := systemdLoadState(layout.ServiceName); err != nil || state != "not-found" {
		return fmt.Errorf("Agent unit remains after uninstall (load_state=%q, error=%v)", state, err)
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
	state, err := systemdLoadState(serviceName)
	if err != nil {
		return false, err
	}
	if state == "not-found" {
		return false, nil
	}
	output, err := exec.Command("systemctl", "show", "--property=ActiveState", "--value", serviceName).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("read systemd ActiveState: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) == "active", nil
}

func nativeServiceE2ERestart(ctx context.Context, serviceName string) error {
	output, err := exec.CommandContext(ctx, "systemctl", "restart", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl restart %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func nativeServiceE2EProcessCounts() (int, int, error) {
	collector, err := processMatchCount("[o]telcol-contrib")
	if err != nil {
		return 0, 0, err
	}
	supervisor, err := processMatchCount("[o]pampsupervisor")
	if err != nil {
		return 0, 0, err
	}
	return collector, supervisor, nil
}

func processMatchCount(pattern string) (int, error) {
	output, err := exec.Command("pgrep", "-af", pattern).CombinedOutput()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return 0, nil
		}
		return 0, fmt.Errorf("pgrep %s: %w: %s", pattern, err, strings.TrimSpace(string(output)))
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}

func systemdLoadState(serviceName string) (string, error) {
	output, err := exec.Command("systemctl", "show", "--property=LoadState", "--value", serviceName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("systemctl show %s: %w: %s", serviceName, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
