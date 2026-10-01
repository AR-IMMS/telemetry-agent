//go:build windows

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
	if !strings.EqualFold(strings.TrimSpace(platform.OS), "windows") {
		return nil, fmt.Errorf(
			"Windows Agent remover requires Windows platform, got %q",
			platform.OS,
		)
	}

	serviceRemover := windowsServiceRemover{
		manager: newWindowsSCMServiceManager(),
	}

	return windowsAgentRemover{
		removeService:   serviceRemover.Remove,
		removeFile:      os.Remove,
		removeDirectory: os.RemoveAll,
	}, nil
}
