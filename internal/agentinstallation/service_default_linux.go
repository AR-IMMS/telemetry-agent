//go:build linux

package agentinstallation

import (
	"fmt"
	"strings"
)

func newDefaultServiceInstaller(
	layout Layout,
) (serviceInstaller, error) {
	if !strings.EqualFold(strings.TrimSpace(layout.Platform), "linux") {
		return nil, fmt.Errorf(
			"Linux service installer requires Linux layout, got %q",
			layout.Platform,
		)
	}

	return newSystemdServiceInstaller(), nil
}
