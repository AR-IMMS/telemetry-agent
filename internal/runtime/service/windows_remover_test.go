package agentinstallation

import (
	"context"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
	"reflect"
	"testing"
)

func TestWindowsAgentRemoverRemovesOwnedResourcesWithDataLast(
	t *testing.T,
) {
	var steps []string

	remover := windowsAgentRemover{
		removeService: func(
			_ context.Context,
			serviceName string,
		) error {
			if serviceName != "ar-imms-telemetry-agent" {
				t.Fatalf("service name = %q", serviceName)
			}

			steps = append(steps, "remove-service")

			return nil
		},
		removeFile: func(path string) error {
			steps = append(steps, "remove-file:"+path)

			return nil
		},
		removeDirectory: func(path string) error {
			steps = append(steps, "remove-directory:"+path)

			return nil
		},
	}

	err := remover.Remove(
		context.Background(),
		servicedomain.AgentInstallation{
			Platform:    "windows",
			ServiceName: "ar-imms-telemetry-agent",
			BinaryPath:  `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
			Ownership: servicedomain.OwnershipRecord{
				Resources: []servicedomain.OwnedResource{
					{
						Kind:       "windows-service",
						Identifier: "ar-imms-telemetry-agent",
					},
					{
						Kind:       "file",
						Identifier: `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
					},
					{
						Kind:       "directory",
						Identifier: `C:\Program Files\AR-IMMS\Telemetry Agent`,
					},
					{
						Kind:       "directory",
						Identifier: `C:\ProgramData\AR-IMMS\Telemetry Agent`,
					},
				},
			},
		},
		`C:\ProgramData\AR-IMMS\Telemetry Agent\state.db`,
	)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if got, want := steps, []string{
		"remove-service",
		`remove-file:C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
		`remove-directory:C:\Program Files\AR-IMMS\Telemetry Agent`,
		`remove-directory:C:\ProgramData\AR-IMMS\Telemetry Agent`,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}
