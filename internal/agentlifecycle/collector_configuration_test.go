package agentlifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestCollectorConfigurationActivatorActivatesEnabledReceivers(
	t *testing.T,
) {
	configRoot := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "otel.yaml")

	basePath := filepath.Join(configRoot, "base.yaml")
	if err := os.WriteFile(basePath, []byte(`
receivers:
  otlp: {}
processors:
  batch: {}
exporters:
  debug: {}
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
`), 0600); err != nil {
		t.Fatalf("write base layer: %v", err)
	}

	windowsPath := filepath.Join(configRoot, "windows.yaml")
	if err := os.WriteFile(windowsPath, []byte(`
receivers:
  hostmetrics: {}
  prometheus/windows_exporter: {}
  prometheus/libre_hardware_monitor: {}
service:
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
      processors: [batch]
      exporters: [debug]
`), 0600); err != nil {
		t.Fatalf("write Windows layer: %v", err)
	}

	catalog, err := dependency.NewCatalog([]dependency.Integration{
		{
			Definition: dependency.Definition{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
				SupportedOS: []string{"windows"},
			},
			CollectorReceiver: "prometheus/windows_exporter",
			Install: func(
				context.Context,
			) (dependency.InstallResult, error) {
				return dependency.InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (dependency.Inspection, error) {
				return dependency.Inspection{}, nil
			},
		},
		{
			Definition: dependency.Definition{
				Name:        "libre-hardware-monitor",
				DisplayName: "Libre Hardware Monitor",
				SupportedOS: []string{"windows"},
			},
			CollectorReceiver: "prometheus/libre_hardware_monitor",
			Install: func(
				context.Context,
			) (dependency.InstallResult, error) {
				return dependency.InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (dependency.Inspection, error) {
				return dependency.Inspection{}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	activator := collectorConfigurationActivator{
		platform: identity.PlatformInfo{
			OS: "windows",
		},
		catalog: catalog,
		layers: []config.Layer{
			{Name: "base", Path: basePath},
			{Name: "windows", Path: windowsPath},
		},
		binaryPath: "otelcol-contrib",
		configPath: targetPath,
		runner:     successfulCollectorRunner{},
	}

	err = activator.Activate(
		context.Background(),
		agentstate.State{
			Dependencies: map[string]agentstate.DependencyState{
				"windows-exporter": {
					Enabled: true,
				},
				"libre-hardware-monitor": {
					Enabled: false,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	rendered, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read activated config: %v", err)
	}

	configText := string(rendered)

	for _, want := range []string{
		"- otlp",
		"- hostmetrics",
		"- prometheus/windows_exporter",
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf(
				"activated config does not contain %q:\n%s",
				want,
				configText,
			)
		}
	}

	if strings.Contains(
		configText,
		"- prometheus/libre_hardware_monitor",
	) {
		t.Fatalf(
			"activated config contains disabled LHM receiver:\n%s",
			configText,
		)
	}
}

func TestCollectorConfigurationUsesCoreGatewayAtActivationTime(t *testing.T) {
	configRoot := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "otel.yaml")
	basePath := filepath.Join(configRoot, "base.yaml")
	if err := os.WriteFile(basePath, []byte(`
receivers:
  otlp: {}
processors:
  batch: {}
exporters:
  otlp:
    endpoint: ${env:OTEL_GATEWAY_ENDPOINT}
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &gatewayValidationRunner{}
	catalog, err := dependency.NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	activator := collectorConfigurationActivator{
		platform:              identity.PlatformInfo{OS: "linux"},
		catalog:               catalog,
		layers:                []config.Layer{{Name: "base", Path: basePath}},
		binaryPath:            "otelcol-contrib",
		configPath:            targetPath,
		validationEnvironment: []string{"OTEL_GATEWAY_ENDPOINT=bootstrap:4317"},
		gatewayRepository:     gatewayRepositoryStub{endpoint: "core.example:4317"},
		runner:                runner,
	}
	if err := activator.Activate(context.Background(), agentstate.State{}); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if runner.environment != "core.example:4317" {
		t.Fatalf("validation endpoint = %q, want Core endpoint", runner.environment)
	}
}

type gatewayValidationRunner struct{ environment string }

func (r *gatewayValidationRunner) Run(_ context.Context, command bootstrap.Command) bootstrap.CommandResult {
	for _, value := range command.Environment {
		if strings.HasPrefix(value, "OTEL_GATEWAY_ENDPOINT=") {
			r.environment = strings.TrimPrefix(value, "OTEL_GATEWAY_ENDPOINT=")
		}
	}
	return bootstrap.CommandResult{ExitCode: 0}
}

type gatewayRepositoryStub struct{ endpoint string }

func (r gatewayRepositoryStub) AcceptGatewaySnapshot(coreconfig.GatewaySnapshot) (coreconfig.Acceptance, error) {
	return coreconfig.Acceptance{}, nil
}

func (r gatewayRepositoryStub) EffectiveGateway() (coreconfig.EffectiveGateway, error) {
	return coreconfig.EffectiveGateway{Owner: coreconfig.CoreOwner, Endpoint: r.endpoint, Generation: 1}, nil
}

type successfulCollectorRunner struct{}

func (successfulCollectorRunner) Run(
	context.Context,
	bootstrap.Command,
) bootstrap.CommandResult {
	return bootstrap.CommandResult{
		ExitCode: 0,
	}
}

func TestApplyDesiredConfigurationActivatesAndAcknowledgesGeneration(
	t *testing.T,
) {
	configRoot := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "otel.yaml")

	basePath := filepath.Join(configRoot, "base.yaml")
	if err := os.WriteFile(basePath, []byte(`
receivers:
  otlp: {}
processors:
  batch: {}
exporters:
  debug: {}
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
`), 0600); err != nil {
		t.Fatalf("write base layer: %v", err)
	}

	windowsPath := filepath.Join(configRoot, "windows.yaml")
	if err := os.WriteFile(windowsPath, []byte(`
receivers:
  hostmetrics: {}
  prometheus/windows_exporter: {}
service:
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
      processors: [batch]
      exporters: [debug]
`), 0600); err != nil {
		t.Fatalf("write Windows layer: %v", err)
	}

	catalog, err := dependency.NewCatalog([]dependency.Integration{
		{
			Definition: dependency.Definition{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
				SupportedOS: []string{"windows"},
			},
			CollectorReceiver: "prometheus/windows_exporter",
			Install: func(
				context.Context,
			) (dependency.InstallResult, error) {
				return dependency.InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (dependency.Inspection, error) {
				return dependency.Inspection{}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	_, err = store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 6
		state.Dependencies = map[string]agentstate.DependencyState{
			"windows-exporter": {
				Enabled: true,
			},
		}

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	err = ApplyDesiredConfiguration(
		context.Background(),
		store,
		CollectorConfigurationOptions{
			Platform: identity.PlatformInfo{
				OS: "windows",
			},
			Catalog: catalog,
			Layers: []config.Layer{
				{Name: "base", Path: basePath},
				{Name: "windows", Path: windowsPath},
			},
			BinaryPath: "otelcol-contrib",
			ConfigPath: targetPath,
			Runner:     successfulCollectorRunner{},
		},
	)
	if err != nil {
		t.Fatalf("ApplyDesiredConfiguration() error = %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActivatedGeneration != 6 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 6",
			state.ActivatedGeneration,
		)
	}
}
