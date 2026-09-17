//go:build windows

package librehardwaremonitor

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// isWindowsAdministrator reports whether the current process token belongs to
// the built-in Windows Administrators group.
func isWindowsAdministrator() (bool, error) {
	administratorsSID, err := windows.CreateWellKnownSid(
		windows.WinBuiltinAdministratorsSid,
	)
	if err != nil {
		return false, fmt.Errorf(
			"create Windows Administrators SID: %w",
			err,
		)
	}

	elevated, err := windows.Token(0).IsMember(administratorsSID)
	if err != nil {
		return false, fmt.Errorf(
			"inspect current Windows token membership: %w",
			err,
		)
	}

	return elevated, nil
}
