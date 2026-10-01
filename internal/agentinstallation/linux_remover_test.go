package agentinstallation

import (
	"context"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestLinuxAgentRemoverRemovesOwnedResourcesWithDataLast(
	t *testing.T,
) {
	var steps []string

	remover := linuxAgentRemover{
		removeService: func(
			_ context.Context,
			serviceName string,
			unitPath string,
		) error {
			if serviceName != "ar-imms-telemetry-agent" {
				t.Fatalf("service name = %q", serviceName)
			}
			if unitPath !=
				"/etc/systemd/system/ar-imms-telemetry-agent.service" {
				t.Fatalf("unit path = %q", unitPath)
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
		agentstate.AgentInstallation{
			Platform:    "linux",
			ServiceName: "ar-imms-telemetry-agent",
			BinaryPath:  "/opt/ar-imms/telemetry-agent/agentctl",
			Ownership: agentstate.OwnershipRecord{
				Resources: []agentstate.OwnedResource{
					{
						Kind:       "systemd-service",
						Identifier: "ar-imms-telemetry-agent",
					},
					{
						Kind: "file",
						Identifier: "/etc/systemd/system/" +
							"ar-imms-telemetry-agent.service",
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
		},
		"/var/lib/ar-imms/telemetry-agent/state.json",
	)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if got, want := steps, []string{
		"remove-service",
		"remove-file:/opt/ar-imms/telemetry-agent/agentctl",
		"remove-directory:/opt/ar-imms/telemetry-agent",
		"remove-directory:/var/lib/ar-imms/telemetry-agent",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

func TestLinuxAgentRemoverRefusesMissingOwnedDataDirectory(
	t *testing.T,
) {
	calls := 0

	remover := linuxAgentRemover{
		removeService: func(
			context.Context,
			string,
			string,
		) error {
			calls++

			return nil
		},
		removeFile: func(string) error {
			calls++

			return nil
		},
		removeDirectory: func(string) error {
			calls++

			return nil
		},
	}

	err := remover.Remove(
		context.Background(),
		agentstate.AgentInstallation{
			Platform:    "linux",
			ServiceName: "ar-imms-telemetry-agent",
			Ownership: agentstate.OwnershipRecord{
				Resources: []agentstate.OwnedResource{
					{
						Kind:       "systemd-service",
						Identifier: "ar-imms-telemetry-agent",
					},
					{
						Kind: "file",
						Identifier: "/etc/systemd/system/" +
							"ar-imms-telemetry-agent.service",
					},
					{
						Kind:       "directory",
						Identifier: "/opt/ar-imms/telemetry-agent",
					},
				},
			},
		},
		"/var/lib/ar-imms/telemetry-agent/state.json",
	)
	if err == nil {
		t.Fatal("Remove() error = nil, want missing data directory error")
	}
	if calls != 0 {
		t.Fatalf("removal calls = %d, want 0", calls)
	}
}
