//go:build linux

package agentinstallation

import (
	"fmt"
	"os"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func newDefaultRemover(
	platform identity.PlatformInfo,
) (Remover, error) {
	if !strings.EqualFold(strings.TrimSpace(platform.OS), "linux") {
		return nil, fmt.Errorf(
			"Linux Agent remover requires Linux platform, got %q",
			platform.OS,
		)
	}

	serviceRemover := newSystemdServiceRemover()

	return linuxAgentRemover{
		removeService:   serviceRemover.Remove,
		removeFile:      os.Remove,
		removeDirectory: os.RemoveAll,
	}, nil
}
