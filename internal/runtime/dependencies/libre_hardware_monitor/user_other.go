//go:build !windows

package librehardwaremonitor

import "fmt"

// currentWindowsUserSID is unavailable outside Windows.
func currentWindowsUserSID() (string, error) {
	return "", fmt.Errorf(
		"current Windows user SID is only available on Windows",
	)
}
