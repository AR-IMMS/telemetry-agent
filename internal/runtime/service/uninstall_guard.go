package agentinstallation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

var ErrInstalledAgentCannotUninstall = errors.New(
	"clean uninstall must be run from an external Agent binary",
)

// ValidateExternalUninstaller refuses clean uninstall when the command is
// running from the Agent binary that is about to be removed.
func ValidateExternalUninstaller(
	currentExecutable string,
	installation agentstate.AgentInstallation,
) error {
	currentExecutable = strings.TrimSpace(currentExecutable)
	if currentExecutable == "" {
		return fmt.Errorf("current Agent executable path is required")
	}

	installedBinary := strings.TrimSpace(installation.BinaryPath)
	if installedBinary == "" {
		return fmt.Errorf("installed Agent binary path is not recorded")
	}

	if sameAgentBinary(
		currentExecutable,
		installedBinary,
		installation.Platform,
	) {
		return ErrInstalledAgentCannotUninstall
	}

	return nil
}

func sameAgentBinary(
	currentExecutable string,
	installedBinary string,
	platform string,
) bool {
	if strings.EqualFold(strings.TrimSpace(platform), "windows") {
		currentExecutable = strings.ReplaceAll(currentExecutable, "/", `\`)
		installedBinary = strings.ReplaceAll(installedBinary, "/", `\`)

		return strings.EqualFold(currentExecutable, installedBinary)
	}

	return currentExecutable == installedBinary
}
