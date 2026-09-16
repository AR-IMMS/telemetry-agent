//go:build !windows

package dependency

import (
	"context"
	"fmt"
)

// inspectWindowsExporterInstallation is unavailable outside Windows. The
// dependency service rejects this integration on those platforms first.
func inspectWindowsExporterInstallation(
	context.Context,
	string,
) (windowsExporterInstallationState, error) {
	return windowsExporterInstallationState{}, fmt.Errorf(
		"Windows Exporter service inspection is unavailable on this operating system",
	)
}
