//go:build !windows

package dependency

import "fmt"

// isWindowsAdministrator is unavailable outside Windows. The dependency
// service rejects Windows Exporter on those platforms before this is used.
func isWindowsAdministrator() (bool, error) {
	return false, fmt.Errorf(
		"Windows Administrator privileges are unavailable on this operating system",
	)
}
