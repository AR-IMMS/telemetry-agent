//go:build !linux && !windows

package agentinstallation

import (
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func newDefaultRemover(
	platform identity.PlatformInfo,
) (Remover, error) {
	return nil, fmt.Errorf(
		"Agent removal is not supported on platform %q",
		platform.OS,
	)
}
