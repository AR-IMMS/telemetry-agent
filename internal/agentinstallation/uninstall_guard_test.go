package agentinstallation

import (
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestValidateExternalUninstallerRejectsInstalledAgentBinary(
	t *testing.T,
) {
	testCases := []struct {
		name      string
		platform  string
		current   string
		installed string
	}{
		{
			name:      "linux",
			platform:  "linux",
			current:   "/opt/ar-imms/telemetry-agent/agentctl",
			installed: "/opt/ar-imms/telemetry-agent/agentctl",
		},
		{
			name:      "windows ignores path case",
			platform:  "windows",
			current:   `c:\program files\ar-imms\telemetry agent\AGENTCTL.EXE`,
			installed: `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateExternalUninstaller(
				test.current,
				agentstate.AgentInstallation{
					Platform:   test.platform,
					BinaryPath: test.installed,
				},
			)

			if !errors.Is(err, ErrInstalledAgentCannotUninstall) {
				t.Fatalf(
					"ValidateExternalUninstaller() error = %v, "+
						"want installed-agent rejection",
					err,
				)
			}
		})
	}
}

func TestValidateExternalUninstallerAllowsExternalBinary(
	t *testing.T,
) {
	err := ValidateExternalUninstaller(
		"/tmp/release/agentctl",
		agentstate.AgentInstallation{
			Platform:   "linux",
			BinaryPath: "/opt/ar-imms/telemetry-agent/agentctl",
		},
	)
	if err != nil {
		t.Fatalf("ValidateExternalUninstaller() error = %v", err)
	}
}
