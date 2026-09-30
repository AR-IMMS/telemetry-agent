package agentinstallation

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestInstallerStagesAgentPersistsStateThenStartsService(
	t *testing.T,
) {
	platform := identity.PlatformInfo{
		OS:           "linux",
		Architecture: "amd64",
	}

	var steps []string
	var gotBootstrapOptions bootstrap.Options

	store := &recordingStateStore{
		state: agentstate.State{
			Dependencies: map[string]agentstate.DependencyState{},
		},
	}

	installer := Installer{
		layoutFor: DefaultLayout,
		copyAgentBinary: func(
			_ context.Context,
			sourcePath string,
			destinationPath string,
		) error {
			steps = append(steps, "copy-agent")

			if sourcePath != "/tmp/release/agentctl" {
				t.Fatalf("source binary = %q", sourcePath)
			}
			if destinationPath !=
				"/opt/ar-imms/telemetry-agent/agentctl" {
				t.Fatalf("destination binary = %q", destinationPath)
			}

			return nil
		},
		bootstrap: func(
			_ context.Context,
			options bootstrap.Options,
		) (bootstrap.Result, error) {
			steps = append(steps, "bootstrap")
			gotBootstrapOptions = options

			return bootstrap.Result{
				Artifact: bootstrap.Artifact{
					Version: "0.160.0",
				},
				BinaryPath: "/opt/ar-imms/telemetry-agent/collector/" +
					"versions/0.160.0/otelcol-contrib",
				ConfigPath: "/var/lib/ar-imms/telemetry-agent/" +
					"collector.yaml",
			}, nil
		},
		newStateStore: func(path string) stateStore {
			if path != "/var/lib/ar-imms/telemetry-agent/state.json" {
				t.Fatalf("state path = %q", path)
			}

			return store
		},
		serviceFor: func(layout Layout) (serviceInstaller, error) {
			return serviceInstallerFunc(func(
				_ context.Context,
				layout Layout,
			) error {
				steps = append(steps, "start-service")

				if layout.ServiceName != "ar-imms-telemetry-agent" {
					t.Fatalf("service name = %q", layout.ServiceName)
				}
				if store.state.Installation == nil {
					t.Fatal("service started before installation was persisted")
				}

				return nil
			}), nil
		},
	}

	configInput := config.RenderInput{
		Platform: platform,
	}

	layout, err := installer.Install(context.Background(), Options{
		Platform:              platform,
		SourceBinaryPath:      "/tmp/release/agentctl",
		ConfigRoot:            "/srv/ar-imms/collector-config",
		GatewayEndpoint:       "gateway.example:4317",
		HealthEndpoint:        "http://127.0.0.1:13133",
		ConfigInput:           configInput,
		ValidationEnvironment: []string{"OTEL_GATEWAY_ENDPOINT=gateway.example:4317"},
		ValidationTimeout:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if layout.StatePath != "/var/lib/ar-imms/telemetry-agent/state.json" {
		t.Fatalf("layout = %#v", layout)
	}

	if got, want := steps,
		[]string{"copy-agent", "bootstrap", "start-service"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}

	if got, want := gotBootstrapOptions.InstallDir,
		"/opt/ar-imms/telemetry-agent/collector"; got != want {
		t.Fatalf("Collector install directory = %q, want %q", got, want)
	}
	if got, want := gotBootstrapOptions.ConfigPath,
		"/var/lib/ar-imms/telemetry-agent/collector.yaml"; got != want {
		t.Fatalf("Collector config path = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(gotBootstrapOptions.ConfigInput, configInput) {
		t.Fatalf(
			"Collector config input = %#v, want %#v",
			gotBootstrapOptions.ConfigInput,
			configInput,
		)
	}

	if got, want := store.state.Collector,
		(agentstate.CollectorContext{
			ConfigRoot: "/srv/ar-imms/collector-config",
			BinaryPath: "/opt/ar-imms/telemetry-agent/collector/" +
				"versions/0.160.0/otelcol-contrib",
			ConfigPath:      "/var/lib/ar-imms/telemetry-agent/collector.yaml",
			GatewayEndpoint: "gateway.example:4317",
			HealthEndpoint:  "http://127.0.0.1:13133",
		}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Collector state = %#v, want %#v", got, want)
	}

	if store.state.Installation == nil {
		t.Fatal("installation = nil")
	}

	if got, want := *store.state.Installation,
		layout.Installation(); !reflect.DeepEqual(got, want) {
		t.Fatalf("installation = %#v, want %#v", got, want)
	}
}

type recordingStateStore struct {
	state agentstate.State
}

func (s *recordingStateStore) Update(
	update agentstate.StateMutation,
) (agentstate.State, error) {
	if err := update(&s.state); err != nil {
		return agentstate.State{}, err
	}

	return s.state, nil
}
