package agentinstallation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestInstallerRequiresOpAMPForNewDefaultSupervisorRuntime(t *testing.T) {
	bootstrapCalled := false
	supervisorInstallCalled := false
	copyAgentCalled := false
	store := &recordingStateStore{state: agentstate.State{}}
	installer := Installer{
		layoutFor: func(platform identity.PlatformInfo) (Layout, error) {
			return Layout{
				Platform:  platform.OS,
				StatePath: filepath.Join(t.TempDir(), "state.db"),
			}, nil
		},
		serviceFor: func(Layout) (serviceInstaller, error) {
			return serviceInstallerFunc(func(context.Context, Layout) error { return nil }), nil
		},
		newStateStore: func(string) stateStore { return store },
		bootstrap: func(context.Context, bootstrap.Options) (bootstrap.Result, error) {
			bootstrapCalled = true
			return bootstrap.Result{}, nil
		},
		installSupervisor: func(context.Context, string, identity.PlatformInfo) (string, error) {
			supervisorInstallCalled = true
			return "/supervisor", nil
		},
		copyAgentBinary: func(context.Context, string, string) error {
			copyAgentCalled = true
			return nil
		},
	}

	_, err := installer.Install(context.Background(), Options{
		Platform:          identity.PlatformInfo{OS: "linux", Architecture: "amd64"},
		SourceBinaryPath:  "/release/agentctl",
		ConfigRoot:        "/release/config",
		GatewayEndpoint:   "gateway.example.test:4317",
		HealthEndpoint:    "http://127.0.0.1:13133",
		ValidationTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "OpAMP server URL is required") {
		t.Fatalf("Install() error = %v, want a missing OpAMP server URL error", err)
	}
	if bootstrapCalled || supervisorInstallCalled || copyAgentCalled {
		t.Fatalf("installation side effects ran before rejecting missing endpoint: bootstrap=%t supervisor=%t copy=%t", bootstrapCalled, supervisorInstallCalled, copyAgentCalled)
	}
	if store.state.Installation != nil {
		t.Fatalf("installation state changed on rejected configuration: %#v", store.state.Installation)
	}
}
