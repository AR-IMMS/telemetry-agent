//go:build windows

package librehardwaremonitor

import (
	"fmt"
	"os/user"
	"strings"
)

// currentWindowsUserSID resolves the SID of the user invoking agentctl.
func currentWindowsUserSID() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf(
			"resolve current Windows user: %w",
			err,
		)
	}

	sid := strings.TrimSpace(current.Uid)
	if sid == "" {
		return "", fmt.Errorf("current Windows user SID is required")
	}

	return sid, nil
}
