package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestManagedDependencyInstallerRequiresBootstrapBeforePhysicalInstall(
	t *testing.T,
) {
	physicalInstallCalls := 0

	install := newManagedDependencyInstaller(
		func() (identity.PlatformInfo, error) {
			return identity.PlatformInfo{
				OS: "windows",
			}, nil
		},
		func(
			context.Context,
			string,
		) (dependency.InstallResult, error) {
			physicalInstallCalls++

			return dependency.InstallResult{
				Name: "windows-exporter",
			}, nil
		},
		bootstrap.OSCommandRunner{},
	)

	_, err := install(
		context.Background(),
		"windows-exporter",
		filepath.Join(t.TempDir(), "missing-state.json"),
	)

	if err == nil || !strings.Contains(
		err.Error(),
		"bootstrap the Collector before managing dependencies",
	) {
		t.Fatalf("install() error = %v, want bootstrap-required error", err)
	}
	if physicalInstallCalls != 0 {
		t.Fatalf(
			"physical installer calls = %d, want 0",
			physicalInstallCalls,
		)
	}
}

func TestManagedDependencyInstallerActivatesEnabledDependencyConfiguration(
	t *testing.T,
) {
	configRoot := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "otel.yaml")

	baseDir := filepath.Join(configRoot, "base")
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		t.Fatalf("create base config directory: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(baseDir, "otel.yaml"),
		[]byte(`
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
`),
		0600,
	); err != nil {
		t.Fatalf("write base config: %v", err)
	}

	profileDir := filepath.Join(configRoot, "profiles")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatalf("create profile config directory: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(profileDir, "laptop.yaml"),
		[]byte("{}\n"),
		0600,
	); err != nil {
		t.Fatalf("write profile config: %v", err)
	}

	windowsDir := filepath.Join(configRoot, "os", "windows")
	if err := os.MkdirAll(windowsDir, 0700); err != nil {
		t.Fatalf("create Windows config directory: %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(windowsDir, "otel.yaml"),
		[]byte(`
receivers:
  hostmetrics: {}
  prometheus/windows_exporter: {}
service:
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
      processors: [batch]
      exporters: [debug]
`),
		0600,
	); err != nil {
		t.Fatalf("write Windows config: %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "state.json")

	store := agentstate.NewFileStore(statePath)

	_, err := store.Update(func(state *agentstate.State) error {
		state.Collector = agentstate.CollectorContext{
			ConfigRoot:      configRoot,
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      targetPath,
			GatewayEndpoint: "127.0.0.1:4317",
		}

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	physicalInstallCalls := 0

	install := newManagedDependencyInstaller(
		func() (identity.PlatformInfo, error) {
			return identity.PlatformInfo{
				OS: "windows",
			}, nil
		},
		func(
			context.Context,
			string,
		) (dependency.InstallResult, error) {
			physicalInstallCalls++

			return dependency.InstallResult{
				Name: "windows-exporter",
				OwnedResources: []agentstate.OwnedResource{
					{
						Kind:       "windows-service",
						Identifier: "windows_exporter",
					},
					{
						Kind:       "file",
						Identifier: targetPath,
					},
				},
			}, nil
		},
		successfulBootstrapRunner{},
	)

	_, err = install(
		context.Background(),
		"windows-exporter",
		statePath,
	)
	if err != nil {
		t.Fatalf("install() error = %v", err)
	}
	if physicalInstallCalls != 1 {
		t.Fatalf(
			"physical installer calls = %d, want 1",
			physicalInstallCalls,
		)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if !state.Dependencies["windows-exporter"].Enabled {
		t.Fatal("windows-exporter Enabled = false, want true")
	}
	ownership := state.Dependencies["windows-exporter"].Ownership

	if ownership == nil {
		t.Fatal("windows-exporter Ownership = nil, want recorded resources")
	}
	if len(ownership.Resources) != 2 {
		t.Fatalf(
			"ownership resources = %#v, want two resources",
			ownership.Resources,
		)
	}
	if ownership.Resources[0].Identifier != "windows_exporter" {
		t.Fatalf(
			"first owned resource = %#v, want windows_exporter service",
			ownership.Resources[0],
		)
	}
	if state.DesiredGeneration != 1 {
		t.Fatalf(
			"DesiredGeneration = %d, want 1",
			state.DesiredGeneration,
		)
	}
	if state.ActivatedGeneration != 1 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 1",
			state.ActivatedGeneration,
		)
	}

	rendered, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read activated config: %v", err)
	}
	if !strings.Contains(
		string(rendered),
		"- prometheus/windows_exporter",
	) {
		t.Fatalf(
			"activated config does not include Windows Exporter:\n%s",
			rendered,
		)
	}
}

type successfulBootstrapRunner struct{}

func (successfulBootstrapRunner) Run(
	context.Context,
	bootstrap.Command,
) bootstrap.CommandResult {
	return bootstrap.CommandResult{
		ExitCode: 0,
	}
}

func TestRunDependencyInstallPassesExplicitStatePathToManagedInstaller(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	statePath := filepath.Join(t.TempDir(), "state.json")
	managedCalls := 0

	exitCode := runDependency(
		context.Background(),
		[]string{
			"install",
			"windows-exporter",
			"--state-path",
			statePath,
		},
		&stdout,
		&stderr,
		dependencies{
			manageDependency: func(
				ctx context.Context,
				name string,
				gotStatePath string,
			) (dependency.InstallResult, error) {
				managedCalls++

				if name != "windows-exporter" {
					t.Fatalf(
						"dependency name = %q, want windows-exporter",
						name,
					)
				}
				if gotStatePath != statePath {
					t.Fatalf(
						"state path = %q, want %q",
						gotStatePath,
						statePath,
					)
				}

				return dependency.InstallResult{
					Name: name,
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(install) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}
	if managedCalls != 1 {
		t.Fatalf(
			"managed installer calls = %d, want 1",
			managedCalls,
		)
	}
	if stdout.String() != "Dependency installed: windows-exporter\n" {
		t.Fatalf(
			"runDependency(install) stdout = %q",
			stdout.String(),
		)
	}
}

func TestDefaultDependenciesConfigureManagedDependencyInstaller(
	t *testing.T,
) {
	deps := defaultDependencies()

	if deps.manageDependency == nil {
		t.Fatal(
			"default dependencies managed installer is nil",
		)
	}
}

func TestManagedDependencyLifecycleRequiresBootstrapBeforeChangingState(
	t *testing.T,
) {
	lifecycle := newManagedDependencyLifecycle(
		nil,
		nil,
	)

	err := lifecycle(
		context.Background(),
		"windows-exporter",
		filepath.Join(t.TempDir(), "missing-state.json"),
		dependencyLifecycleDisable,
	)

	if err == nil || !strings.Contains(
		err.Error(),
		"bootstrap the Collector before managing dependencies",
	) {
		t.Fatalf("lifecycle() error = %v, want bootstrap-required error", err)
	}
}

func TestManagedDependencyLifecycleDisablesDependencyAndActivatesConfiguration(
	t *testing.T,
) {
	configRoot := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "otel.yaml")

	writeConfig := func(relativePath string, content string) {
		path := filepath.Join(configRoot, relativePath)

		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatalf("create config directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatalf("write config %q: %v", relativePath, err)
		}
	}

	writeConfig("base/otel.yaml", `
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
`)

	writeConfig("profiles/laptop.yaml", "{}\n")

	writeConfig("os/windows/otel.yaml", `
receivers:
  hostmetrics: {}
  prometheus/windows_exporter: {}
service:
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
      processors: [batch]
      exporters: [debug]
`)

	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	_, err := store.Update(func(state *agentstate.State) error {
		state.Collector = agentstate.CollectorContext{
			ConfigRoot:      configRoot,
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      targetPath,
			GatewayEndpoint: "127.0.0.1:4317",
		}
		state.DesiredGeneration = 1
		state.ActivatedGeneration = 1
		state.AppliedGeneration = 1
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

	lifecycle := newManagedDependencyLifecycle(
		func() (identity.PlatformInfo, error) {
			return identity.PlatformInfo{
				OS: "windows",
			}, nil
		},
		successfulBootstrapRunner{},
	)

	err = lifecycle(
		context.Background(),
		"windows-exporter",
		statePath,
		dependencyLifecycleDisable,
	)
	if err != nil {
		t.Fatalf("lifecycle(disable) error = %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	dependency := state.Dependencies["windows-exporter"]
	if dependency.Enabled {
		t.Fatal("windows-exporter Enabled = true, want false")
	}
	if dependency.PendingTeardown == nil {
		t.Fatal("PendingTeardown = nil, want disable teardown")
	}
	if dependency.PendingTeardown.Action != agentstate.TeardownActionDisable {
		t.Fatalf(
			"PendingTeardown.Action = %q, want %q",
			dependency.PendingTeardown.Action,
			dependencyLifecycleDisable,
		)
	}
	if dependency.PendingTeardown.Generation != 2 {
		t.Fatalf(
			"PendingTeardown.Generation = %d, want 2",
			dependency.PendingTeardown.Generation,
		)
	}
	if state.ActivatedGeneration != 2 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 2",
			state.ActivatedGeneration,
		)
	}

	rendered, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read activated config: %v", err)
	}
	if strings.Contains(
		string(rendered),
		"- prometheus/windows_exporter",
	) {
		t.Fatalf(
			"activated metrics pipeline still uses Windows Exporter:\n%s",
			rendered,
		)
	}
}

func TestManagedDependencyLifecycleEnablesOwnedDependencyAndActivatesConfiguration(
	t *testing.T,
) {
	configRoot := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "otel.yaml")

	writeConfig := func(relativePath string, content string) {
		path := filepath.Join(configRoot, relativePath)

		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatalf("create config directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatalf("write config %q: %v", relativePath, err)
		}
	}

	writeConfig("base/otel.yaml", `
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
`)

	writeConfig("profiles/laptop.yaml", "{}\n")

	writeConfig("os/windows/otel.yaml", `
receivers:
  hostmetrics: {}
  prometheus/windows_exporter: {}
service:
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
      processors: [batch]
      exporters: [debug]
`)

	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	ownership := []agentstate.OwnedResource{
		{
			Kind:       "windows-service",
			Identifier: "windows_exporter",
		},
	}

	_, err := store.Update(func(state *agentstate.State) error {
		state.Collector = agentstate.CollectorContext{
			ConfigRoot:      configRoot,
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      targetPath,
			GatewayEndpoint: "127.0.0.1:4317",
		}
		state.DesiredGeneration = 1
		state.ActivatedGeneration = 1
		state.AppliedGeneration = 1
		state.Dependencies = map[string]agentstate.DependencyState{
			"windows-exporter": {
				Enabled:   false,
				Ownership: &agentstate.OwnershipRecord{Resources: ownership},
			},
		}

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	enableCalls := 0

	lifecycle := newManagedDependencyLifecycle(
		func() (identity.PlatformInfo, error) {
			return identity.PlatformInfo{OS: "windows"}, nil
		},
		successfulBootstrapRunner{},
		func(
			ctx context.Context,
			name string,
			resources []agentstate.OwnedResource,
		) error {
			enableCalls++

			if ctx == nil {
				t.Fatal("enable context = nil")
			}
			if name != "windows-exporter" {
				t.Fatalf("enable name = %q, want windows-exporter", name)
			}
			if !reflect.DeepEqual(resources, ownership) {
				t.Fatalf(
					"enable ownership = %#v, want %#v",
					resources,
					ownership,
				)
			}

			return nil
		},
	)

	err = lifecycle(
		context.Background(),
		"windows-exporter",
		statePath,
		dependencyLifecycleEnable,
	)
	if err != nil {
		t.Fatalf("lifecycle(enable) error = %v", err)
	}

	if enableCalls != 1 {
		t.Fatalf("enable calls = %d, want 1", enableCalls)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	dependency := state.Dependencies["windows-exporter"]
	if !dependency.Enabled {
		t.Fatal("windows-exporter Enabled = false, want true")
	}
	if dependency.PendingTeardown != nil {
		t.Fatalf(
			"PendingTeardown = %#v, want nil",
			dependency.PendingTeardown,
		)
	}
	if state.DesiredGeneration != 2 {
		t.Fatalf(
			"DesiredGeneration = %d, want 2",
			state.DesiredGeneration,
		)
	}
	if state.ActivatedGeneration != 2 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 2",
			state.ActivatedGeneration,
		)
	}

	rendered, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read activated config: %v", err)
	}
	if !strings.Contains(
		string(rendered),
		"- prometheus/windows_exporter",
	) {
		t.Fatalf(
			"activated metrics pipeline does not use Windows Exporter:\n%s",
			rendered,
		)
	}
}

func TestDefaultDependenciesConfigureManagedDependencyLifecycle(
	t *testing.T,
) {
	deps := defaultDependencies()

	if deps.manageDependencyLifecycle == nil {
		t.Fatal(
			"default dependencies managed lifecycle manager is nil",
		)
	}
}

func TestManagedDependencyLifecycleRejectsEnableWithoutOwnership(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	_, err := store.Update(func(state *agentstate.State) error {
		state.Collector = agentstate.CollectorContext{
			ConfigRoot:      "configs",
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      "otel.yaml",
			GatewayEndpoint: "127.0.0.1:4317",
		}
		state.DesiredGeneration = 1
		state.ActivatedGeneration = 1
		state.AppliedGeneration = 1
		state.Dependencies = map[string]agentstate.DependencyState{
			"windows-exporter": {
				Enabled: false,
			},
		}

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	enableCalls := 0

	lifecycle := newManagedDependencyLifecycle(
		nil,
		nil,
		func(
			context.Context,
			string,
			[]agentstate.OwnedResource,
		) error {
			enableCalls++

			return nil
		},
	)

	err = lifecycle(
		context.Background(),
		"windows-exporter",
		statePath,
		dependencyLifecycleEnable,
	)

	if err == nil || !strings.Contains(
		err.Error(),
		"disabled without Agent-owned resources",
	) {
		t.Fatalf(
			"lifecycle(enable) error = %v, want ownership-required error",
			err,
		)
	}
	if enableCalls != 0 {
		t.Fatalf("enable calls = %d, want 0", enableCalls)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.Dependencies["windows-exporter"].Enabled {
		t.Fatal("windows-exporter Enabled = true, want false")
	}
	if state.DesiredGeneration != 1 {
		t.Fatalf(
			"DesiredGeneration = %d, want 1",
			state.DesiredGeneration,
		)
	}
}
