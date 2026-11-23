package agentinstallation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	config "github.com/ar-imms/telemetry-agent/internal/runtime/collector/configuration"
	collectordomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/domain"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	opampdomain "github.com/ar-imms/telemetry-agent/internal/runtime/opamp/domain"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
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
			Dependencies: map[string]dependencydomain.DependencyState{},
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
		installSupervisor: func(_ context.Context, installDir string, _ identity.PlatformInfo) (string, error) {
			steps = append(steps, "install-supervisor")
			if installDir != "/opt/ar-imms/telemetry-agent/opampsupervisor" {
				t.Fatalf("Supervisor install directory = %q", installDir)
			}
			return "/opt/ar-imms/telemetry-agent/opampsupervisor/versions/0.160.0/opampsupervisor", nil
		},
		newStateStore: func(path string) stateStore {
			if path != "/var/lib/ar-imms/telemetry-agent/state.db" {
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
		OpAMPServerURL:        "ws://127.0.0.1:4320/v1/opamp",
		ConfigInput:           configInput,
		ValidationEnvironment: []string{"OTEL_GATEWAY_ENDPOINT=gateway.example:4317"},
		ValidationTimeout:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if layout.StatePath != "/var/lib/ar-imms/telemetry-agent/state.db" {
		t.Fatalf("layout = %#v", layout)
	}

	if got, want := steps,
		[]string{"bootstrap", "install-supervisor", "copy-agent", "start-service"}; !reflect.DeepEqual(got, want) {
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
		(collectordomain.CollectorContext{
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
	if got := store.state.Supervisor; got.BinaryPath == "" || !got.Enabled || got.OpAMPServerURL != "ws://127.0.0.1:4320/v1/opamp" {
		t.Fatalf("Supervisor state = %#v, want default Supervisor owner for a new install", got)
	}

	if got, want := *store.state.Installation,
		layout.Installation(); !reflect.DeepEqual(got, want) {
		t.Fatalf("installation = %#v, want %#v", got, want)
	}
}

func TestInstallerAcceptsWrappedMissingStateForNewInstallation(t *testing.T) {
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	store := &recordingStateStore{loadErr: fmt.Errorf("load Agent state: %w", os.ErrNotExist)}
	serviceStarted := false
	installer := Installer{
		layoutFor:       DefaultLayout,
		copyAgentBinary: func(context.Context, string, string) error { return nil },
		bootstrap: func(context.Context, bootstrap.Options) (bootstrap.Result, error) {
			return bootstrap.Result{BinaryPath: "/collector", ConfigPath: "/collector.yaml"}, nil
		},
		installSupervisor: func(context.Context, string, identity.PlatformInfo) (string, error) {
			return "", nil
		},
		newStateStore: func(string) stateStore { return store },
		serviceFor: func(Layout) (serviceInstaller, error) {
			return serviceInstallerFunc(func(context.Context, Layout) error {
				serviceStarted = true
				return nil
			}), nil
		},
	}

	_, err := installer.Install(context.Background(), Options{
		Platform: platform, SourceBinaryPath: "/release/agentctl", ConfigRoot: "/release/config",
		GatewayEndpoint: "127.0.0.1:4317", HealthEndpoint: "http://127.0.0.1:13133",
		DisableSupervisor: true, ConfigInput: config.RenderInput{Platform: platform},
		ValidationTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Install() error = %v, want a wrapped missing state to start a new installation", err)
	}
	if !serviceStarted {
		t.Fatal("service was not started after initializing a new installation")
	}
}

func TestInstallerRequiresExplicitIssuerWhenRemoteSupervisorEndpointChanges(t *testing.T) {
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	store := &recordingStateStore{state: agentstate.State{
		Supervisor: opampdomain.SupervisorContext{
			OpAMPServerURL:   "wss://core.example.test/v1/opamp",
			CredentialSource: opampdomain.CredentialSourceCore,
			Enabled:          true,
		},
	}}
	bootstrapCalled := false
	installer := Installer{
		layoutFor:       DefaultLayout,
		copyAgentBinary: func(context.Context, string, string) error { return nil },
		bootstrap: func(context.Context, bootstrap.Options) (bootstrap.Result, error) {
			bootstrapCalled = true
			return bootstrap.Result{BinaryPath: "/collector", ConfigPath: "/collector.yaml"}, nil
		},
		installSupervisor: func(context.Context, string, identity.PlatformInfo) (string, error) { return "/supervisor", nil },
		newStateStore:     func(string) stateStore { return store },
		serviceFor: func(Layout) (serviceInstaller, error) {
			return serviceInstallerFunc(func(context.Context, Layout) error { return nil }), nil
		},
	}

	_, err := installer.Install(context.Background(), Options{
		Platform: platform, SourceBinaryPath: "/release/agentctl", ConfigRoot: "/release/config",
		GatewayEndpoint: "127.0.0.1:4317", HealthEndpoint: "http://127.0.0.1:13133",
		OpAMPServerURL: "wss://fleet.example.test/v1/opamp", ConfigInput: config.RenderInput{Platform: platform},
		ValidationTimeout: time.Second,
	})
	if err == nil {
		t.Fatal("Install() accepted a changed remote endpoint without an explicit credential issuer")
	}
	if bootstrapCalled {
		t.Fatal("Install() began mutating installation before rejecting an ambiguous issuer")
	}
	if store.state.Supervisor.OpAMPServerURL != "wss://core.example.test/v1/opamp" || store.state.Supervisor.CredentialSource != opampdomain.CredentialSourceCore {
		t.Fatalf("ambiguous install changed existing Supervisor state: %+v", store.state.Supervisor)
	}
}

func TestInstallerExplicitLegacyRollbackDisablesManagedSupervisor(t *testing.T) {
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	layout, err := DefaultLayout(platform)
	if err != nil {
		t.Fatal(err)
	}
	layout.CollectorConfigurationPath = filepath.Join(t.TempDir(), "collector.yaml")
	endpoint := "wss://opamp.example.test/v1/opamp"
	store := &recordingStateStore{state: agentstate.State{
		Installation: &servicedomain.AgentInstallation{},
		Supervisor: opampdomain.SupervisorContext{
			BinaryPath:     "/opt/ar-imms/telemetry-agent/opampsupervisor/supervisor",
			OpAMPServerURL: endpoint,
			Enabled:        true,
		},
		Dependencies: map[string]dependencydomain.DependencyState{},
	}}
	serviceRunning := true
	var serviceSteps []string
	installer := Installer{
		layoutFor: func(identity.PlatformInfo) (Layout, error) { return layout, nil },
		copyAgentBinary: func(context.Context, string, string) error {
			serviceSteps = append(serviceSteps, "copy")
			if serviceRunning {
				t.Fatal("Agent binary was replaced while the installed service was running")
			}
			return nil
		},
		bootstrap: func(_ context.Context, options bootstrap.Options) (bootstrap.Result, error) {
			if err := os.WriteFile(options.ConfigPath, []byte("candidate\n"), 0o600); err != nil {
				return bootstrap.Result{}, err
			}
			return bootstrap.Result{BinaryPath: "/collector", ConfigPath: "/collector.yaml"}, nil
		},
		installSupervisor: func(context.Context, string, identity.PlatformInfo) (string, error) {
			t.Fatal("legacy rollback must not install or activate a Supervisor")
			return "", nil
		},
		newStateStore: func(string) stateStore { return store },
		serviceFor: func(Layout) (serviceInstaller, error) {
			return stoppingServiceInstaller{
				stop: func(context.Context, Layout) error {
					serviceSteps = append(serviceSteps, "stop")
					serviceRunning = false
					return nil
				},
				install: func(context.Context, Layout) error {
					serviceSteps = append(serviceSteps, "restart")
					serviceRunning = true
					return nil
				},
			}, nil
		},
	}
	_, err = installer.Install(context.Background(), Options{
		Platform: platform, SourceBinaryPath: "/tmp/agentctl", ConfigRoot: "/etc/ar-imms",
		GatewayEndpoint: "127.0.0.1:4317", HealthEndpoint: "http://127.0.0.1:13133",
		DisableSupervisor: true, ConfigInput: config.RenderInput{Platform: platform},
		ValidationTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Install() rollback error = %v", err)
	}
	if store.state.Supervisor.Enabled || store.state.Supervisor.OpAMPServerURL != "" {
		t.Fatalf("Supervisor state after rollback = %#v, want disabled with no endpoint", store.state.Supervisor)
	}
	if store.state.Supervisor.BinaryPath == "" {
		t.Fatal("rollback discarded the pinned Supervisor path needed to support a later migration")
	}
	if !reflect.DeepEqual(serviceSteps, []string{"stop", "copy", "restart"}) {
		t.Fatalf("service and binary sequence = %v, want stop, copy, restart", serviceSteps)
	}
}

func TestInstallerRestartsExistingServiceWhenStatePersistenceFails(t *testing.T) {
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	layout, err := DefaultLayout(platform)
	if err != nil {
		t.Fatal(err)
	}
	layout.CollectorConfigurationPath = filepath.Join(t.TempDir(), "collector.yaml")
	persistErr := fmt.Errorf("state database unavailable")
	store := &recordingStateStore{
		state: agentstate.State{
			Installation: &servicedomain.AgentInstallation{},
			Dependencies: map[string]dependencydomain.DependencyState{},
		},
		updateErr: persistErr,
	}
	serviceRunning := true
	var serviceSteps []string
	installer := Installer{
		layoutFor: func(identity.PlatformInfo) (Layout, error) { return layout, nil },
		copyAgentBinary: func(context.Context, string, string) error {
			serviceSteps = append(serviceSteps, "copy")
			if serviceRunning {
				t.Fatal("Agent binary was replaced while the installed service was running")
			}
			return nil
		},
		bootstrap: func(_ context.Context, options bootstrap.Options) (bootstrap.Result, error) {
			if err := os.WriteFile(options.ConfigPath, []byte("candidate\n"), 0o600); err != nil {
				return bootstrap.Result{}, err
			}
			return bootstrap.Result{BinaryPath: "/collector", ConfigPath: "/collector.yaml"}, nil
		},
		installSupervisor: func(context.Context, string, identity.PlatformInfo) (string, error) {
			return "", nil
		},
		newStateStore: func(string) stateStore { return store },
		serviceFor: func(Layout) (serviceInstaller, error) {
			return stoppingServiceInstaller{
				stop: func(context.Context, Layout) error {
					serviceSteps = append(serviceSteps, "stop")
					serviceRunning = false
					return nil
				},
				install: func(context.Context, Layout) error {
					serviceSteps = append(serviceSteps, "restart")
					serviceRunning = true
					return nil
				},
			}, nil
		},
	}

	_, err = installer.Install(context.Background(), Options{
		Platform: platform, SourceBinaryPath: "/tmp/agentctl", ConfigRoot: "/etc/ar-imms",
		GatewayEndpoint: "127.0.0.1:4317", HealthEndpoint: "http://127.0.0.1:13133",
		DisableSupervisor: true, ConfigInput: config.RenderInput{Platform: platform},
		ValidationTimeout: time.Second,
	})
	if !errors.Is(err, persistErr) {
		t.Fatalf("Install() error = %v, want wrapped state persistence error", err)
	}
	if serviceRunning != true {
		t.Fatal("existing Agent service remained stopped after state persistence failed")
	}
	if !reflect.DeepEqual(serviceSteps, []string{"stop", "copy", "restart"}) {
		t.Fatalf("service and binary sequence = %v, want stop, copy, restart", serviceSteps)
	}
}

func TestInstallerRestoresPreviousOwnerAfterTargetServiceStartFailure(t *testing.T) {
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	activationErr := errors.New("target service start failed")
	configPath := filepath.Join(t.TempDir(), "collector.yaml")
	if err := os.WriteFile(configPath, []byte("previous collector configuration\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousConfigSecurity := setConfigSecurityForTest(t, configPath)
	layout := Layout{
		Platform:                    "linux",
		InstallDirectory:            "/new/install",
		DataDirectory:               "/new/data",
		AgentBinaryPath:             "/new/agentctl",
		CollectorConfigurationPath:  configPath,
		SupervisorInstallDirectory:  "/new/supervisor",
		SupervisorConfigurationPath: "/new/supervisor.yaml",
		SupervisorStateDirectory:    "/new/supervisor-state",
		StatePath:                   filepath.Join(t.TempDir(), "state.db"),
		ServiceName:                 DefaultAgentServiceName,
	}
	previousState := agentstate.State{
		Collector: collectordomain.CollectorContext{
			BinaryPath: "/old/collector", ConfigPath: configPath,
			GatewayEndpoint: "gateway.old:4317", HealthEndpoint: "http://127.0.0.1:13133",
		},
		Supervisor: opampdomain.SupervisorContext{BinaryPath: "/old/opampsupervisor"},
		Installation: &servicedomain.AgentInstallation{
			Platform: "linux", ServiceName: "ar-imms-telemetry-agent", BinaryPath: "/old/agentctl",
			Ownership: servicedomain.OwnershipRecord{Resources: []servicedomain.OwnedResource{{Kind: "file", Identifier: "/old/agentctl"}}},
		},
		Dependencies: map[string]dependencydomain.DependencyState{},
	}
	store := &recordingStateStore{state: previousState}
	serviceRunning := true
	installCount := 0
	var steps []string
	installer := Installer{
		layoutFor: func(identity.PlatformInfo) (Layout, error) { return layout, nil },
		copyAgentBinary: func(context.Context, string, string) error {
			steps = append(steps, "copy-agent")
			if serviceRunning {
				t.Fatal("Agent binary was replaced while a service was running")
			}
			return nil
		},
		bootstrap: func(_ context.Context, options bootstrap.Options) (bootstrap.Result, error) {
			steps = append(steps, "bootstrap")
			if options.ConfigPath == configPath {
				t.Fatal("update bootstrap rendered directly over the active Collector configuration")
			}
			activeConfig, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(activeConfig), "previous collector configuration\n"; got != want {
				t.Fatalf("active Collector config during staging = %q, want prior config %q", got, want)
			}
			if err := os.WriteFile(options.ConfigPath, []byte("target collector configuration\n"), 0o640); err != nil {
				return bootstrap.Result{}, err
			}
			return bootstrap.Result{BinaryPath: "/new/collector", ConfigPath: options.ConfigPath}, nil
		},
		installSupervisor: func(context.Context, string, identity.PlatformInfo) (string, error) {
			steps = append(steps, "install-supervisor")
			return "/new/opampsupervisor", nil
		},
		newStateStore: func(string) stateStore { return store },
		serviceFor: func(Layout) (serviceInstaller, error) {
			return stoppingServiceInstaller{
				stop: func(context.Context, Layout) error {
					if serviceRunning {
						steps = append(steps, "stop")
						serviceRunning = false
					}
					return nil
				},
				install: func(context.Context, Layout) error {
					installCount++
					if installCount == 1 {
						steps = append(steps, "start-target")
						if got := captureConfigSecurityForTest(t, configPath); got != previousConfigSecurity {
							t.Fatalf("target Collector config security = %q, want preserved prior security %q", got, previousConfigSecurity)
						}
						activeConfig, err := os.ReadFile(configPath)
						if err != nil {
							t.Fatal(err)
						}
						if got, want := string(activeConfig), "target collector configuration\n"; got != want {
							t.Fatalf("target service started with Collector config %q, want %q", got, want)
						}
						if !store.state.Supervisor.Enabled {
							t.Fatal("target service started before Supervisor ownership was persisted")
						}
						serviceRunning = true
						return activationErr
					}
					steps = append(steps, "restart-previous")
					if !reflect.DeepEqual(store.state, previousState) {
						t.Fatalf("rollback service state = %#v, want previous owner state %#v", store.state, previousState)
					}
					serviceRunning = true
					return nil
				},
			}, nil
		},
	}

	_, err := installer.Install(context.Background(), Options{
		Platform: platform, SourceBinaryPath: "/tmp/new-agentctl", ConfigRoot: "/etc/ar-imms",
		GatewayEndpoint: "gateway.new:4317", HealthEndpoint: "http://127.0.0.1:13133",
		OpAMPServerURL: "wss://opamp.example.test/v1/opamp", CredentialSource: opampdomain.CredentialSourceCore,
		ConfigInput: config.RenderInput{Platform: platform}, ValidationTimeout: time.Second,
	})
	if !errors.Is(err, activationErr) {
		t.Fatalf("Install() error = %v, want target service start error", err)
	}
	if !serviceRunning {
		t.Fatal("previous Agent service was not restarted after rollback")
	}
	if !reflect.DeepEqual(store.state, previousState) {
		t.Fatalf("Install() error = %v; transition steps = %v; persisted Agent state = %#v, want previous owner state %#v", err, steps, store.state, previousState)
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(configBytes), "previous collector configuration\n"; got != want {
		t.Fatalf("active Collector config = %q, want restored previous config %q", got, want)
	}
	if got, want := steps, []string{"bootstrap", "install-supervisor", "stop", "copy-agent", "start-target", "stop", "restart-previous"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("transition steps = %v, want %v", got, want)
	}
}

type stoppingServiceInstaller struct {
	stop    func(context.Context, Layout) error
	install func(context.Context, Layout) error
}

func (s stoppingServiceInstaller) Stop(ctx context.Context, layout Layout) error {
	return s.stop(ctx, layout)
}

func (s stoppingServiceInstaller) Install(ctx context.Context, layout Layout) error {
	return s.install(ctx, layout)
}

type recordingStateStore struct {
	state     agentstate.State
	loadErr   error
	updateErr error
}

func (s *recordingStateStore) Load() (agentstate.State, error) { return s.state, s.loadErr }

func (s *recordingStateStore) Update(
	update agentstate.StateMutation,
) (agentstate.State, error) {
	if s.updateErr != nil {
		return agentstate.State{}, s.updateErr
	}
	if err := update(&s.state); err != nil {
		return agentstate.State{}, err
	}

	return s.state, nil
}
