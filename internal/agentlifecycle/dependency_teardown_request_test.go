package agentlifecycle

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestRequestAllDependencyUninstallsUsesOneGenerationForAllPersistedDependencies(
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
`), 0o600); err != nil {
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
`), 0o600); err != nil {
		t.Fatalf("write Windows layer: %v", err)
	}

	catalog, err := dependency.NewCatalog([]dependency.Integration{
		testTeardownIntegration(
			"windows-exporter",
			"Windows Exporter",
			"prometheus/windows_exporter",
		),
		testTeardownIntegration(
			"libre-hardware-monitor",
			"Libre Hardware Monitor",
			"prometheus/libre_hardware_monitor",
		),
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)
	if err := store.Save(agentstate.State{
		DesiredGeneration:   7,
		ActivatedGeneration: 7,
		AppliedGeneration:   7,
		Dependencies: map[string]agentstate.DependencyState{
			"windows-exporter": {
				Enabled: true,
			},
			"libre-hardware-monitor": {
				Enabled: true,
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	runner := &countingCollectorRunner{}

	changed, err := RequestAllDependencyUninstalls(
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
			Runner:     runner,
		},
	)
	if err != nil {
		t.Fatalf("RequestAllDependencyUninstalls() error = %v", err)
	}

	if got, want := changed,
		[]string{"libre-hardware-monitor", "windows-exporter"}; !reflect.DeepEqual(
		got,
		want,
	) {
		t.Fatalf("changed dependencies = %v, want %v", got, want)
	}

	if runner.calls != 1 {
		t.Fatalf(
			"Collector validation calls = %d, want 1",
			runner.calls,
		)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if state.DesiredGeneration != 8 {
		t.Fatalf(
			"DesiredGeneration = %d, want 8",
			state.DesiredGeneration,
		)
	}
	if state.ActivatedGeneration != 8 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 8",
			state.ActivatedGeneration,
		)
	}
	if state.AppliedGeneration != 7 {
		t.Fatalf(
			"AppliedGeneration = %d, want 7",
			state.AppliedGeneration,
		)
	}

	for _, name := range changed {
		dependency := state.Dependencies[name]

		if dependency.Enabled {
			t.Fatalf("%s Enabled = true, want false", name)
		}
		if dependency.PendingTeardown == nil {
			t.Fatalf("%s PendingTeardown = nil", name)
		}
		if dependency.PendingTeardown.Action !=
			agentstate.TeardownActionUninstall {
			t.Fatalf(
				"%s PendingTeardown.Action = %q, want uninstall",
				name,
				dependency.PendingTeardown.Action,
			)
		}
		if dependency.PendingTeardown.Generation != 8 {
			t.Fatalf(
				"%s PendingTeardown.Generation = %d, want 8",
				name,
				dependency.PendingTeardown.Generation,
			)
		}
	}
}

func testTeardownIntegration(
	name string,
	displayName string,
	receiver string,
) dependency.Integration {
	return dependency.Integration{
		Definition: dependency.Definition{
			Name:        name,
			DisplayName: displayName,
			SupportedOS: []string{"windows"},
		},
		CollectorReceiver: receiver,
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
	}
}

type countingCollectorRunner struct {
	calls int
}

func (r *countingCollectorRunner) Run(
	context.Context,
	bootstrap.Command,
) bootstrap.CommandResult {
	r.calls++

	return bootstrap.CommandResult{
		ExitCode: 0,
	}
}
