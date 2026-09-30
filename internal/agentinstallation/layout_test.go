package agentinstallation

import (
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestDefaultLayoutReturnsOwnedPlatformPaths(t *testing.T) {
	tests := []struct {
		name     string
		platform identity.PlatformInfo
		want     Layout
	}{
		{
			name: "linux",
			platform: identity.PlatformInfo{
				OS: "linux",
			},
			want: Layout{
				Platform:                   "linux",
				InstallDirectory:           "/opt/ar-imms/telemetry-agent",
				DataDirectory:              "/var/lib/ar-imms/telemetry-agent",
				AgentBinaryPath:            "/opt/ar-imms/telemetry-agent/agentctl",
				CollectorInstallDirectory:  "/opt/ar-imms/telemetry-agent/collector",
				CollectorConfigurationPath: "/var/lib/ar-imms/telemetry-agent/collector.yaml",
				StatePath:                  "/var/lib/ar-imms/telemetry-agent/state.json",
				ServiceName:                "ar-imms-telemetry-agent",
			},
		},
		{
			name: "windows",
			platform: identity.PlatformInfo{
				OS: "windows",
			},
			want: Layout{
				Platform:                   "windows",
				InstallDirectory:           `C:\Program Files\AR-IMMS\Telemetry Agent`,
				DataDirectory:              `C:\ProgramData\AR-IMMS\Telemetry Agent`,
				AgentBinaryPath:            `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
				CollectorInstallDirectory:  `C:\Program Files\AR-IMMS\Telemetry Agent\collector`,
				CollectorConfigurationPath: `C:\ProgramData\AR-IMMS\Telemetry Agent\collector.yaml`,
				StatePath:                  `C:\ProgramData\AR-IMMS\Telemetry Agent\state.json`,
				ServiceName:                "ar-imms-telemetry-agent",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DefaultLayout(test.platform)
			if err != nil {
				t.Fatalf("DefaultLayout() error = %v", err)
			}

			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("DefaultLayout() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestLayoutInstallationReturnsConcreteOwnedResources(t *testing.T) {
	layout, err := DefaultLayout(identity.PlatformInfo{
		OS: "linux",
	})
	if err != nil {
		t.Fatalf("DefaultLayout() error = %v", err)
	}

	got := layout.Installation()

	want := agentstate.AgentInstallation{
		Platform:    "linux",
		ServiceName: "ar-imms-telemetry-agent",
		Ownership: agentstate.OwnershipRecord{
			Resources: []agentstate.OwnedResource{
				{
					Kind:       "systemd-service",
					Identifier: "ar-imms-telemetry-agent",
				},
				{
					Kind:       "file",
					Identifier: "/opt/ar-imms/telemetry-agent/agentctl",
				},
				{
					Kind:       "directory",
					Identifier: "/opt/ar-imms/telemetry-agent",
				},
				{
					Kind:       "directory",
					Identifier: "/var/lib/ar-imms/telemetry-agent",
				},
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Installation() = %#v, want %#v", got, want)
	}
}
