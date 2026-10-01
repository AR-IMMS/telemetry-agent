package agentinstallation

import (
	"context"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestWindowsServiceInstallerEnsuresAndRestartsAgentService(
	t *testing.T,
) {
	layout, err := DefaultLayout(identity.PlatformInfo{
		OS: "windows",
	})
	if err != nil {
		t.Fatalf("DefaultLayout() error = %v", err)
	}

	var calls []string
	var gotDefinition windowsServiceDefinition

	installer := windowsServiceInstaller{
		manager: windowsServiceManagerFunc{
			ensure: func(
				_ context.Context,
				definition windowsServiceDefinition,
			) error {
				calls = append(calls, "ensure")
				gotDefinition = definition

				return nil
			},
			restart: func(
				_ context.Context,
				serviceName string,
			) error {
				calls = append(calls, "restart")

				if serviceName != "ar-imms-telemetry-agent" {
					t.Fatalf("service name = %q", serviceName)
				}

				return nil
			},
		},
	}

	if err := installer.Install(context.Background(), layout); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	wantDefinition := windowsServiceDefinition{
		Name:           "ar-imms-telemetry-agent",
		DisplayName:    "AR-IMMS Telemetry Agent",
		ExecutablePath: `C:\Program Files\AR-IMMS\Telemetry Agent\agentctl.exe`,
		Args: []string{
			"service",
			"--state-path",
			`C:\ProgramData\AR-IMMS\Telemetry Agent\state.json`,
		},
	}

	if !reflect.DeepEqual(gotDefinition, wantDefinition) {
		t.Fatalf(
			"service definition = %#v, want %#v",
			gotDefinition,
			wantDefinition,
		)
	}

	if got, want := calls,
		[]string{"ensure", "restart"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestWindowsServiceRemoverRemovesAgentService(
	t *testing.T,
) {
	var calls []string

	remover := windowsServiceRemover{
		manager: windowsServiceManagerFunc{
			remove: func(
				_ context.Context,
				serviceName string,
			) error {
				calls = append(calls, "remove")

				if serviceName != "ar-imms-telemetry-agent" {
					t.Fatalf("service name = %q", serviceName)
				}

				return nil
			},
		},
	}

	if err := remover.Remove(
		context.Background(),
		"ar-imms-telemetry-agent",
	); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if got, want := calls, []string{"remove"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}
