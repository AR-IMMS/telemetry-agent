//go:build !linux && !windows

package agentinstallation

import "fmt"

func newDefaultServiceInstaller(
	layout Layout,
) (serviceInstaller, error) {
	return nil, fmt.Errorf(
		"Agent service installation is not supported on platform %q",
		layout.Platform,
	)
}
