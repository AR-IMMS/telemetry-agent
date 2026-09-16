//go:build windows

package dependency

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// isWindowsAdministrator reports whether the effective current process token
// belongs to the built-in Administrators group.
func isWindowsAdministrator() (bool, error) {
	administratorsSID, err := windows.CreateWellKnownSid(
		windows.WinBuiltinAdministratorsSid,
	)
	if err != nil {
		return false, fmt.Errorf(
			"resolve built-in Administrators SID: %w",
			err,
		)
	}

	elevated, err := windows.Token(0).IsMember(administratorsSID)
	if err != nil {
		return false, fmt.Errorf(
			"inspect current process token membership: %w",
			err,
		)
	}

	return elevated, nil
}
