package agentinstallation

import (
	"fmt"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const DefaultAgentServiceName = "ar-imms-telemetry-agent"

// Layout defines the concrete paths and service identity owned by a packaged
// Agent installation on one supported platform.
type Layout struct {
	Platform                   string
	InstallDirectory           string
	DataDirectory              string
	AgentBinaryPath            string
	CollectorInstallDirectory  string
	CollectorConfigurationPath string
	StatePath                  string
	ServiceName                string
}

// DefaultLayout returns the standard Agent-owned layout for a supported
// platform. User-supplied configuration roots are intentionally not included:
// they remain user-owned.
func DefaultLayout(platform identity.PlatformInfo) (Layout, error) {
	switch strings.ToLower(strings.TrimSpace(platform.OS)) {
	case "linux":
		return Layout{
			Platform:                   "linux",
			InstallDirectory:           "/opt/ar-imms/telemetry-agent",
			DataDirectory:              "/var/lib/ar-imms/telemetry-agent",
			AgentBinaryPath:            "/opt/ar-imms/telemetry-agent/agentctl",
			CollectorInstallDirectory:  "/opt/ar-imms/telemetry-agent/collector",
			CollectorConfigurationPath: "/var/lib/ar-imms/telemetry-agent/collector.yaml",
			StatePath:                  "/var/lib/ar-imms/telemetry-agent/state.json",
			ServiceName:                DefaultAgentServiceName,
		}, nil

	case "windows":
		return Layout{
			Platform:                   "windows",
			InstallDirectory:           `C:\Program Files\AR-IMMS\Telemetry Agent`,
			DataDirectory:              `C:\ProgramData\AR-IMMS\Telemetry Agent`,
			AgentBinaryPath:            `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
			CollectorInstallDirectory:  `C:\Program Files\AR-IMMS\Telemetry Agent\collector`,
			CollectorConfigurationPath: `C:\ProgramData\AR-IMMS\Telemetry Agent\collector.yaml`,
			StatePath:                  `C:\ProgramData\AR-IMMS\Telemetry Agent\state.json`,
			ServiceName:                DefaultAgentServiceName,
		}, nil

	default:
		return Layout{}, fmt.Errorf(
			"Agent installation is not supported on platform %q",
			platform.OS,
		)
	}
}

// Installation returns the concrete Agent-owned resources represented by this
// resolved installation layout.
func (l Layout) Installation() agentstate.AgentInstallation {
	serviceKind := "systemd-service"
	if strings.EqualFold(l.Platform, "windows") {
		serviceKind = "windows-service"
	}

	return agentstate.AgentInstallation{
		Platform:    l.Platform,
		ServiceName: l.ServiceName,
		Ownership: agentstate.OwnershipRecord{
			Resources: []agentstate.OwnedResource{
				{
					Kind:       serviceKind,
					Identifier: l.ServiceName,
				},
				{
					Kind:       "file",
					Identifier: l.AgentBinaryPath,
				},
				{
					Kind:       "directory",
					Identifier: l.InstallDirectory,
				},
				{
					Kind:       "directory",
					Identifier: l.DataDirectory,
				},
			},
		},
	}
}
