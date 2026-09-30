//go:build windows

package agentinstallation

import (
	"fmt"
	"strings"
)

func newDefaultServiceInstaller(
	layout Layout,
) (serviceInstaller, error) {
	if !strings.EqualFold(strings.TrimSpace(layout.Platform), "windows") {
		return nil, fmt.Errorf(
			"Windows service installer requires Windows layout, got %q",
			layout.Platform,
		)
	}

	return windowsServiceInstaller{
		manager: newWindowsSCMServiceManager(),
	}, nil
}
