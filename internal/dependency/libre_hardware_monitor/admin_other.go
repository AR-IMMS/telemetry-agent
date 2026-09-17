//go:build !windows

package librehardwaremonitor

import "fmt"

// isWindowsAdministrator is unavailable outside Windows.
func isWindowsAdministrator() (bool, error) {
	return false, fmt.Errorf(
		"Libre Hardware Monitor installation is supported only on Windows",
	)
}
