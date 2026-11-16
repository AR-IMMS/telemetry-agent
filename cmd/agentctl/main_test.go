package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation"
	config "github.com/ar-imms/telemetry-agent/internal/runtime/collector/configuration"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/supervisor"
	"github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

func TestBootstrapPassesOrderedLinuxLayersAndValidationOptions(t *testing.T) {
	var gotOptions bootstrap.Options
	statePath := filepath.Join(t.TempDir(), "state.json")

	deps := testDependencies(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	deps.runBootstrap = func(
		_ context.Context,
		options bootstrap.Options,
		_ bootstrap.Downloader,
		_ bootstrap.CommandRunner,
	) (bootstrap.Result, error) {
		gotOptions = options
		return bootstrap.Result{Artifact: bootstrap.Artifact{Version: "0.160.0"}}, nil
	}

	code, stdout, stderr := runForTest(t, []string{
		"bootstrap",
		"--config-root", "test-configs",
		"--install-dir", "test-install",
		"--config-path", "test-output/otel.yaml",
		"--state-path", statePath,
		"--validation-endpoint", "gateway.test:4317",
		"--timeout", "45s",
	}, deps)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", code, stderr)
	}
	if stdout == "" {
		t.Fatal("success output is empty")
	}
	if gotOptions.InstallDir != "test-install" || gotOptions.ConfigPath != "test-output/otel.yaml" {
		t.Fatalf("paths = %+v, want flag values", gotOptions)
	}
	if gotOptions.Platform.OS != "linux" {
		t.Fatalf("platform OS = %q, want linux", gotOptions.Platform.OS)
	}
	if gotOptions.ValidationTimeout != 45*time.Second {
		t.Fatalf("validation timeout = %v, want 45s", gotOptions.ValidationTimeout)
	}
	if got := strings.Join(gotOptions.ValidationEnvironment, ","); got != "OTEL_GATEWAY_ENDPOINT=gateway.test:4317" {
		t.Fatalf("validation environment = %q, want validation-only endpoint", got)
	}

	gotLayers := gotOptions.ConfigInput.Layers
	wantPaths := []string{
		filepath.Join("test-configs", "base", "otel.yaml"),
		filepath.Join("test-configs", "profiles", "laptop.yaml"),
		filepath.Join("test-configs", "os", "linux", "otel.yaml"),
	}
	if len(gotLayers) != len(wantPaths) {
		t.Fatalf("layer count = %d, want %d", len(gotLayers), len(wantPaths))
	}
	for index, want := range wantPaths {
		if gotLayers[index].Path != want {
			t.Fatalf("layer %d path = %q, want %q", index, gotLayers[index].Path, want)
		}
	}
}

func TestBootstrapUsesWindowsConfigurationLayer(t *testing.T) {
	deps := testDependencies(identity.PlatformInfo{OS: "windows", Architecture: "amd64"})
	var gotOptions bootstrap.Options
	deps.runBootstrap = func(
		_ context.Context,
		options bootstrap.Options,
		_ bootstrap.Downloader,
		_ bootstrap.CommandRunner,
	) (bootstrap.Result, error) {
		gotOptions = options
		return bootstrap.Result{Artifact: bootstrap.Artifact{Version: "0.160.0"}}, nil
	}

	statePath := filepath.Join(t.TempDir(), "state.json")

	code, _, stderr := runForTest(
		t,
		append(
			validBootstrapArguments(),
			"--state-path",
			statePath,
		),
		deps,
	)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", code, stderr)
	}
	got := gotOptions.ConfigInput.Layers[2].Path
	want := filepath.Join("test-configs", "os", "windows", "otel.yaml")
	if got != want {
		t.Fatalf("OS layer = %q, want %q", got, want)
	}
}

func TestBootstrapDefaultsValidationEndpointAndTimeout(t *testing.T) {
	deps := testDependencies(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	var gotOptions bootstrap.Options
	statePath := filepath.Join(t.TempDir(), "state.json")
	deps.runBootstrap = func(
		_ context.Context,
		options bootstrap.Options,
		_ bootstrap.Downloader,
		_ bootstrap.CommandRunner,
	) (bootstrap.Result, error) {
		gotOptions = options
		return bootstrap.Result{Artifact: bootstrap.Artifact{Version: "0.160.0"}}, nil
	}

	code, _, stderr := runForTest(
		t,
		append(
			validBootstrapArguments(),
			"--state-path",
			statePath,
		),
		deps,
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", code, stderr)
	}
	if got := strings.Join(gotOptions.ValidationEnvironment, ","); got != "OTEL_GATEWAY_ENDPOINT=127.0.0.1:4317" {
		t.Fatalf("validation environment = %q, want default endpoint", got)
	}
	if gotOptions.ValidationTimeout != 2*time.Minute {
		t.Fatalf("validation timeout = %v, want 2m", gotOptions.ValidationTimeout)
	}
}

func TestBootstrapRejectsUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "unsupported root command",
			args: []string{"not-a-command"},
		},
		{name: "missing config root", args: []string{"bootstrap", "--install-dir", "test-install", "--config-path", "test-output/otel.yaml"}},
		{name: "missing install dir", args: []string{"bootstrap", "--config-root", "test-configs", "--config-path", "test-output/otel.yaml"}},
		{name: "missing config path", args: []string{"bootstrap", "--config-root", "test-configs", "--install-dir", "test-install"}},
		{name: "unknown flag", args: append(validBootstrapArguments(), "--unknown")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runForTest(t, test.args, testDependencies(identity.PlatformInfo{OS: "linux", Architecture: "amd64"}))
			if code == 0 {
				t.Fatal("exit code = 0, want usage error")
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want no success output", stdout)
			}
			if !strings.Contains(stderr, "usage") {
				t.Fatalf("stderr = %q, want concise usage error", stderr)
			}
		})
	}
}

func TestBootstrapFailurePreservesErrorAndSuppressesSuccessOutput(t *testing.T) {
	deps := testDependencies(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	deps.runBootstrap = func(
		_ context.Context,
		_ bootstrap.Options,
		_ bootstrap.Downloader,
		_ bootstrap.CommandRunner,
	) (bootstrap.Result, error) {
		return bootstrap.Result{}, errors.New("render Collector configuration: invalid profile")
	}

	args := append(
		validBootstrapArguments(),
		"--state-path",
		filepath.Join(t.TempDir(), "state.json"),
	)

	code, stdout, stderr := runForTest(t, args, deps)

	if code == 0 {
		t.Fatal("exit code = 0, want bootstrap failure")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want no success output", stdout)
	}
	if !strings.Contains(stderr, "render Collector configuration: invalid profile") {
		t.Fatalf("stderr = %q, want bootstrap error context", stderr)
	}
}

func TestBootstrapSuccessPrintsInstallationResult(t *testing.T) {
	deps := testDependencies(
		identity.PlatformInfo{OS: "linux", Architecture: "amd64"},
	)
	deps.runBootstrap = func(
		_ context.Context,
		_ bootstrap.Options,
		_ bootstrap.Downloader,
		_ bootstrap.CommandRunner,
	) (bootstrap.Result, error) {
		return bootstrap.Result{
			Artifact:   bootstrap.Artifact{Version: "0.160.0"},
			BinaryPath: "test-install/versions/otelcol-contrib",
			ConfigPath: "test-output/otel.yaml",
			Reused:     true,
		}, nil
	}

	statePath := filepath.Join(t.TempDir(), "state.json")

	code, stdout, stderr := runForTest(
		t,
		append(
			validBootstrapArguments(),
			"--state-path",
			statePath,
		),
		deps,
	)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", code, stderr)
	}
	for _, want := range []string{
		"Collector version: 0.160.0",
		"Collector binary path: test-install/versions/otelcol-contrib",
		"Final configuration path: test-output/otel.yaml",
		"Collector installation reused: true",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func validBootstrapArguments() []string {
	return []string{
		"bootstrap",
		"--config-root", "test-configs",
		"--install-dir", "test-install",
		"--config-path", "test-output/otel.yaml",
	}
}

func runForTest(t *testing.T, args []string, deps dependencies) (int, string, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), args, &stdout, &stderr, deps)

	return code, stdout.String(), stderr.String()
}

func testDependencies(platform identity.PlatformInfo) dependencies {
	return dependencies{
		collectPlatform: func() (identity.PlatformInfo, error) {
			return platform, nil
		},
	}
}

func TestRunPassesExplicitStatePathToRuntime(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	wantStatePath := filepath.Join(t.TempDir(), "state.json")
	var gotStatePath string

	exitCode := run(
		context.Background(),
		[]string{
			"run",
			"--state-path",
			wantStatePath,
		},
		&stdout,
		&stderr,
		dependencies{
			prepareInstallationIdentity: skipInstallationIdentityPreparation,
			runCollectorRuntime: func(
				ctx context.Context,
				statePath string,
				_ agentlifecycle.DependencyTeardownFunc,
			) error {
				gotStatePath = statePath

				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"run() exit code = %d, want 0; stderr = %s",
			exitCode,
			stderr.String(),
		)
	}
	if gotStatePath != wantStatePath {
		t.Fatalf(
			"runtime state path = %q, want %q",
			gotStatePath,
			wantStatePath,
		)
	}
	if stdout.String() != "Collector stopped\n" {
		t.Fatalf(
			"stdout = %q, want Collector stopped message",
			stdout.String(),
		)
	}
}

func TestMonitoringHierarchyAliasesPreserveLegacyUseCaseOutput(t *testing.T) {
	deps := testDependencies(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	tests := []struct {
		name    string
		legacy  []string
		newPath []string
	}{
		{name: "instance configure", legacy: []string{"integration", "configure"}, newPath: []string{"monitoring", "instance", "configure"}},
		{name: "instance update", legacy: []string{"integration", "update"}, newPath: []string{"monitoring", "instance", "update"}},
		{name: "instance enable", legacy: []string{"integration", "enable"}, newPath: []string{"monitoring", "instance", "enable"}},
		{name: "instance disable", legacy: []string{"integration", "disable"}, newPath: []string{"monitoring", "instance", "disable"}},
		{name: "instance delete", legacy: []string{"integration", "delete"}, newPath: []string{"monitoring", "instance", "delete"}},
		{name: "instance list", legacy: []string{"integration", "list"}, newPath: []string{"monitoring", "instance", "list"}},
		{name: "instance inspect", legacy: []string{"integration", "inspect"}, newPath: []string{"monitoring", "instance", "inspect"}},
		{name: "instance status", legacy: []string{"integration", "status"}, newPath: []string{"monitoring", "instance", "status"}},
		{name: "instance history", legacy: []string{"integration", "history"}, newPath: []string{"monitoring", "instance", "history"}},
		{name: "instance retry", legacy: []string{"integration", "retry"}, newPath: []string{"monitoring", "instance", "retry"}},
		{name: "profile list", legacy: []string{"integration", "catalog", "list"}, newPath: []string{"monitoring", "profile", "list"}},
		{name: "profile inspect", legacy: []string{"integration", "catalog", "inspect"}, newPath: []string{"monitoring", "profile", "inspect"}},
		{name: "template create", legacy: []string{"integration", "template", "create"}, newPath: []string{"monitoring", "template", "create"}},
		{name: "template update", legacy: []string{"integration", "template", "update"}, newPath: []string{"monitoring", "template", "update"}},
		{name: "template list", legacy: []string{"integration", "template", "list"}, newPath: []string{"monitoring", "template", "list"}},
		{name: "template inspect", legacy: []string{"integration", "template", "inspect"}, newPath: []string{"monitoring", "template", "inspect"}},
		{name: "template enable", legacy: []string{"integration", "template", "enable"}, newPath: []string{"monitoring", "template", "enable"}},
		{name: "template disable", legacy: []string{"integration", "template", "disable"}, newPath: []string{"monitoring", "template", "disable"}},
		{name: "template delete", legacy: []string{"integration", "template", "delete"}, newPath: []string{"monitoring", "template", "delete"}},
		{name: "proposal list", legacy: []string{"discovery", "proposals", "list"}, newPath: []string{"monitoring", "proposal", "list"}},
		{name: "proposal inspect", legacy: []string{"discovery", "proposals", "inspect"}, newPath: []string{"monitoring", "proposal", "inspect"}},
		{name: "proposal dismiss", legacy: []string{"discovery", "proposals", "dismiss"}, newPath: []string{"monitoring", "proposal", "dismiss"}},
		{name: "proposal approve", legacy: []string{"discovery", "proposals", "approve"}, newPath: []string{"monitoring", "proposal", "approve"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statePath := filepath.Join(t.TempDir(), "state.db")
			probe := []string{"--state-path", statePath, "--compatibility-probe"}
			legacyCode, legacyOut, legacyErr := runForTest(t, append(append([]string{}, test.legacy...), probe...), deps)
			aliasCode, aliasOut, aliasErr := runForTest(t, append(append([]string{}, test.newPath...), probe...), deps)
			if aliasCode != legacyCode || aliasOut != legacyOut || aliasErr != legacyErr {
				t.Fatalf("alias result (%d, %q, %q) differs from legacy result (%d, %q, %q)", aliasCode, aliasOut, aliasErr, legacyCode, legacyOut, legacyErr)
			}
			if aliasCode == 0 || aliasErr == "" {
				t.Fatalf("compatibility probe unexpectedly succeeded: exit=%d stdout=%q stderr=%q", aliasCode, aliasOut, aliasErr)
			}
		})
	}
}
func TestMonitoringAliasValidListOutputMatchesLegacy(t *testing.T) {
	deps := testDependencies(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	tests := []struct {
		name   string
		legacy []string
		alias  []string
	}{
		{name: "instance list", legacy: []string{"integration", "list"}, alias: []string{"monitoring", "instance", "list"}},
		{name: "profile list", legacy: []string{"integration", "catalog", "list", "--output", "json"}, alias: []string{"monitoring", "profile", "list", "--output", "json"}},
		{name: "profile inspect", legacy: []string{"integration", "catalog", "inspect", "--id", "redis", "--version", "1.0.0", "--output", "json"}, alias: []string{"monitoring", "profile", "inspect", "--id", "redis", "--version", "1.0.0", "--output", "json"}},
		{name: "template list", legacy: []string{"integration", "template", "list"}, alias: []string{"monitoring", "template", "list"}},
		{name: "proposal list", legacy: []string{"discovery", "proposals", "list"}, alias: []string{"monitoring", "proposal", "list"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statePath := filepath.Join(t.TempDir(), "state.db")
			legacyArgs := append(append([]string{}, test.legacy...), "--state-path", statePath)
			aliasArgs := append(append([]string{}, test.alias...), "--state-path", statePath)
			legacyCode, legacyOut, legacyErr := runForTest(t, legacyArgs, deps)
			aliasCode, aliasOut, aliasErr := runForTest(t, aliasArgs, deps)
			if aliasCode != legacyCode || aliasOut != legacyOut || aliasErr != legacyErr {
				t.Fatalf("alias result (%d, %q, %q) differs from legacy result (%d, %q, %q)", aliasCode, aliasOut, aliasErr, legacyCode, legacyOut, legacyErr)
			}
		})
	}
}
func TestRunRejectsMissingGatewayEndpoint(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{
			"run",
			"--collector-path",
			"C:/agent/bin/otelcol-contrib.exe",
			"--config-path",
			"C:/agent/config/otel.yaml",
		},
		&stdout,
		&stderr,
		dependencies{
			runSupervisor: func(
				context.Context,
				supervisor.Options,
			) error {
				t.Fatal("supervisor must not run with missing gateway endpoint")
				return nil
			},
		},
	)

	if exitCode != 2 {
		t.Fatalf(
			"run() exit code = %d, want 2; stderr = %s",
			exitCode,
			stderr.String(),
		)
	}
}

func TestDependencyInstallInvokesInstaller(t *testing.T) {
	var gotName string

	deps := dependencies{
		installDependency: func(
			_ context.Context,
			name string,
		) (dependency.InstallResult, error) {
			gotName = name

			return dependency.InstallResult{
				Name:   "windows-exporter",
				Reused: false,
			}, nil
		},
	}

	code, stdout, stderr := runForTest(
		t,
		[]string{
			"dependency",
			"install",
			"windows-exporter",
		},
		deps,
	)

	if code != 0 {
		t.Fatalf(
			"exit code = %d, want 0; stderr = %s",
			code,
			stderr,
		)
	}

	if gotName != "windows-exporter" {
		t.Fatalf(
			"installer dependency name = %q, want windows-exporter",
			gotName,
		)
	}

	if !strings.Contains(
		stdout,
		"Dependency installed: windows-exporter",
	) {
		t.Fatalf(
			"stdout = %q, want installation result",
			stdout,
		)
	}
}

func TestDefaultDependenciesConfigureDependencyInstaller(t *testing.T) {
	deps := defaultDependencies()

	if deps.installDependency == nil {
		t.Fatal("default dependency installer is nil")
	}
}

func TestDefaultDependencyCatalogIncludesBuiltInIntegrations(
	t *testing.T,
) {
	tests := []struct {
		name          string
		supportedOS   string
		unsupportedOS string
	}{
		{
			name:          "windows-exporter",
			supportedOS:   "windows",
			unsupportedOS: "linux",
		},
		{
			name:          "node-exporter",
			supportedOS:   "linux",
			unsupportedOS: "windows",
		},
		{
			name:          "libre-hardware-monitor",
			supportedOS:   "windows",
			unsupportedOS: "linux",
		},
	}

	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			integration, found := catalog.Find(test.name)

			if !found {
				t.Fatalf(
					"defaultDependencyCatalog() does not contain %q",
					test.name,
				)
			}
			if integration.Install == nil {
				t.Fatalf("integration %q installer is nil", test.name)
			}
			if integration.Definition.DisplayName == "" {
				t.Fatalf("integration %q display name is empty", test.name)
			}
			if integration.Definition.Description == "" {
				t.Fatalf("integration %q description is empty", test.name)
			}
			if !integration.Definition.SupportsOS(test.supportedOS) {
				t.Fatalf(
					"integration %q does not support %q",
					test.name,
					test.supportedOS,
				)
			}
			if integration.Definition.SupportsOS(test.unsupportedOS) {
				t.Fatalf(
					"integration %q unexpectedly supports %q",
					test.name,
					test.unsupportedOS,
				)
			}
		})
	}
}

func TestDefaultDependenciesConfigureDependencySelector(t *testing.T) {
	deps := defaultDependencies()

	if deps.selectDependencies == nil {
		t.Fatal("default dependency selector is nil")
	}
}

func TestDefaultDependencyCatalogConfiguresNodeExporterInspector(
	t *testing.T,
) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	integration, found := catalog.Find("node-exporter")
	if !found {
		t.Fatal("node-exporter integration was not found")
	}
	if integration.Inspect == nil {
		t.Fatal("node-exporter inspector is nil")
	}
}

func TestDefaultDependencyCatalogConfiguresWindowsExporterInspector(
	t *testing.T,
) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	integration, found := catalog.Find("windows-exporter")
	if !found {
		t.Fatal("windows-exporter integration was not found")
	}
	if integration.Inspect == nil {
		t.Fatal("windows-exporter inspector is nil")
	}
}

func TestDefaultDependencyCatalogConfiguresLibreHardwareMonitorInspector(
	t *testing.T,
) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	integration, found := catalog.Find("libre-hardware-monitor")
	if !found {
		t.Fatal("libre-hardware-monitor integration was not found")
	}
	if integration.Inspect == nil {
		t.Fatal("libre-hardware-monitor inspector is nil")
	}
}

func TestDefaultDependencyCatalogConfiguresCollectorReceivers(t *testing.T) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	for name, want := range map[string]string{
		"windows-exporter":       "prometheus/windows_exporter",
		"node-exporter":          "prometheus/node_exporter",
		"libre-hardware-monitor": "prometheus/libre_hardware_monitor",
	} {
		integration, found := catalog.Find(name)
		if !found {
			t.Fatalf("catalog does not contain %q", name)
		}
		if integration.CollectorReceiver != want {
			t.Fatalf(
				"%s CollectorReceiver = %q, want %q",
				name,
				integration.CollectorReceiver,
				want,
			)
		}
	}
}

func TestDependencyMetricsReceiverLayerIncludesOnlyEnabledCurrentPlatform(
	t *testing.T,
) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	layer, err := dependencyMetricsReceiverLayer(
		catalog,
		"windows",
		agentstate.State{
			Dependencies: map[string]agentstate.DependencyState{
				"windows-exporter": {
					Enabled: true,
				},
				"libre-hardware-monitor": {
					Enabled: false,
				},
				"node-exporter": {
					Enabled: true,
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("dependencyMetricsReceiverLayer() error = %v", err)
	}

	want := config.MetricsReceiverLayer([]string{
		"otlp",
		"hostmetrics",
		"prometheus/windows_exporter",
	})

	if !reflect.DeepEqual(layer, want) {
		t.Fatalf(
			"dependencyMetricsReceiverLayer() = %#v, want %#v",
			layer,
			want,
		)
	}
}

func TestRunBootstrapPersistsCollectorContextAndManagedReceiverLayer(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var gotOptions bootstrap.Options

	configRoot := t.TempDir()
	installDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "otel.yaml")
	statePath := filepath.Join(t.TempDir(), "state.json")

	exitCode := runBootstrap(
		context.Background(),
		[]string{
			"--config-root", configRoot,
			"--install-dir", installDir,
			"--config-path", configPath,
			"--validation-endpoint", "gateway.example:4317",
			"--gateway-endpoint", "runtime.example:4317",
			"--state-path", statePath,
		},
		&stdout,
		&stderr,
		dependencies{
			collectPlatform: func() (identity.PlatformInfo, error) {
				return identity.PlatformInfo{
					OS: "windows",
				}, nil
			},
			runBootstrap: func(
				ctx context.Context,
				options bootstrap.Options,
				downloader bootstrap.Downloader,
				runner bootstrap.CommandRunner,
			) (bootstrap.Result, error) {
				gotOptions = options

				return bootstrap.Result{
					BinaryPath: `C:\AR-IMMS\otelcol-contrib.exe`,
					ConfigPath: configPath,
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runBootstrap() exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	wantLayer := config.MetricsReceiverLayer([]string{
		"otlp",
		"hostmetrics",
	})

	if !reflect.DeepEqual(gotOptions.ConfigInput.InlineLayers, []config.InlineLayer{
		wantLayer,
	}) {
		t.Fatalf(
			"bootstrap InlineLayers = %#v, want %#v",
			gotOptions.ConfigInput.InlineLayers,
			[]config.InlineLayer{wantLayer},
		)
	}

	if got := strings.Join(gotOptions.ValidationEnvironment, ","); got !=
		"OTEL_GATEWAY_ENDPOINT=gateway.example:4317" {
		t.Fatalf(
			"validation environment = %q, want validation-only endpoint",
			got,
		)
	}

	state, err := sqlitefixture.NewStore(statePath).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantContext := agentstate.CollectorContext{
		ConfigRoot:      configRoot,
		BinaryPath:      `C:\AR-IMMS\otelcol-contrib.exe`,
		ConfigPath:      configPath,
		GatewayEndpoint: "runtime.example:4317",
		HealthEndpoint:  defaultHealthEndpoint,
	}

	if !reflect.DeepEqual(state.Collector, wantContext) {
		t.Fatalf(
			"persisted Collector = %#v, want %#v",
			state.Collector,
			wantContext,
		)
	}
}

func TestRunBootstrapDoesNotPersistCollectorContextWhenBootstrapFails(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	statePath := filepath.Join(t.TempDir(), "state.json")

	exitCode := runBootstrap(
		context.Background(),
		[]string{
			"--config-root", t.TempDir(),
			"--install-dir", t.TempDir(),
			"--config-path", filepath.Join(t.TempDir(), "otel.yaml"),
			"--state-path", statePath,
		},
		&stdout,
		&stderr,
		dependencies{
			collectPlatform: func() (identity.PlatformInfo, error) {
				return identity.PlatformInfo{
					OS: "windows",
				}, nil
			},
			runBootstrap: func(
				context.Context,
				bootstrap.Options,
				bootstrap.Downloader,
				bootstrap.CommandRunner,
			) (bootstrap.Result, error) {
				return bootstrap.Result{}, errors.New("validation failed")
			},
		},
	)

	if exitCode != 1 {
		t.Fatalf(
			"runBootstrap() exit code = %d, want 1; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	_, err := sqlitefixture.NewStore(statePath).Load()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state Load() error = %v, want not-exist error", err)
	}
}

func TestRunCollectorPassesExplicitStatePath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	wantStatePath := filepath.Join(t.TempDir(), "state.json")
	var gotStatePath string

	exitCode := runCollector(
		context.Background(),
		[]string{"--state-path", wantStatePath},
		&stdout,
		&stderr,
		dependencies{
			prepareInstallationIdentity: skipInstallationIdentityPreparation,
			runCollectorRuntime: func(
				ctx context.Context,
				statePath string,
				_ agentlifecycle.DependencyTeardownFunc,
			) error {
				gotStatePath = statePath

				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runCollector() exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}
	if gotStatePath != wantStatePath {
		t.Fatalf(
			"state path = %q, want %q",
			gotStatePath,
			wantStatePath,
		)
	}
}

func TestRunCollectorRejectsEmptyStatePath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runCollector(
		context.Background(),
		[]string{"--state-path", "   "},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf("runCollector() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(
		stderr.String(),
		"--state-path must not be empty",
	) {
		t.Fatalf(
			"stderr = %q, want empty-state-path error",
			stderr.String(),
		)
	}
}

func TestDefaultDependencyCatalogConfiguresNodeExporterTeardown(
	t *testing.T,
) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	integration, found := catalog.Find("node-exporter")
	if !found {
		t.Fatal("node-exporter integration was not found")
	}
	if integration.Teardown == nil {
		t.Fatal("node-exporter Teardown = nil")
	}
}

func TestDefaultDependencyCatalogConfiguresWindowsExporterTeardown(
	t *testing.T,
) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	integration, found := catalog.Find("windows-exporter")
	if !found {
		t.Fatal("windows-exporter integration was not found")
	}
	if integration.Teardown == nil {
		t.Fatal("windows-exporter Teardown = nil")
	}
}

func TestDefaultDependencyCatalogConfiguresLHMTeardown(t *testing.T) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	integration, found := catalog.Find("libre-hardware-monitor")
	if !found {
		t.Fatal("LHM integration not found")
	}

	if integration.Teardown == nil {
		t.Fatal("LHM teardown = nil, want configured teardown")
	}
}

func TestDefaultDependencyCatalogConfiguresEnablers(
	t *testing.T,
) {
	catalog, err := defaultDependencyCatalog()
	if err != nil {
		t.Fatalf("defaultDependencyCatalog() error = %v", err)
	}

	for _, name := range []string{
		"windows-exporter",
		"node-exporter",
		"libre-hardware-monitor",
	} {
		integration, found := catalog.Find(name)
		if !found {
			t.Fatalf("catalog does not contain %q", name)
		}
		if integration.Enable == nil {
			t.Fatalf("catalog integration %q enabler is nil", name)
		}
	}
}

func TestRunCollectorServesRuntimeHealthSnapshot(
	t *testing.T,
) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var gotListenAddress string
	var servedSnapshot agenthealth.Snapshot

	exitCode := runCollector(
		context.Background(),
		nil,
		&stdout,
		&stderr,
		dependencies{
			prepareInstallationIdentity: skipInstallationIdentityPreparation,
			listenStatus: func(
				network string,
				address string,
			) (net.Listener, error) {
				if network != "tcp" {
					t.Fatalf("status network = %q, want tcp", network)
				}

				gotListenAddress = address

				return listener, nil
			},
			serveStatus: func(
				ctx context.Context,
				_ net.Listener,
				provider agenthealth.Provider,
			) error {
				<-ctx.Done()
				servedSnapshot = provider.Snapshot()

				return nil
			},
			runCollectorRuntimeWithHealth: func(
				context.Context,
				string,
				dependencyTeardownFunc,
				agenthealth.ObservationReporter,
			) error {
				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runCollector() exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	if gotListenAddress != defaultStatusListenAddress {
		t.Fatalf(
			"status listen address = %q, want %q",
			gotListenAddress,
			defaultStatusListenAddress,
		)
	}

	if servedSnapshot.CollectorState != agenthealth.CollectorStateStarting {
		t.Fatalf(
			"served Collector state = %q, want %q",
			servedSnapshot.CollectorState,
			agenthealth.CollectorStateStarting,
		)
	}

	if stdout.String() != "Collector stopped\n" {
		t.Fatalf("runCollector() stdout = %q", stdout.String())
	}
}

func TestRunStatusRendersLiveAgentHealth(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var gotEndpoint string

	exitCode := run(
		context.Background(),
		[]string{"status"},
		&stdout,
		&stderr,
		dependencies{
			fetchHealthStatus: func(
				ctx context.Context,
				endpoint string,
			) (agenthealth.Snapshot, error) {
				gotEndpoint = endpoint

				return agenthealth.Snapshot{
					Status:              agenthealth.StatusHealthy,
					CollectorState:      agenthealth.CollectorStateReady,
					DesiredGeneration:   4,
					ActivatedGeneration: 4,
					AppliedGeneration:   4,
					PendingTeardowns:    0,
					Assessment: agenthealth.Assessment{
						State: agenthealth.EvidenceUnknown,
						Components: []agenthealth.ComponentEvidence{
							{Component: agenthealth.ComponentAgentRuntime, State: agenthealth.EvidenceHealthy, Freshness: agenthealth.FreshnessFresh},
							{Component: agenthealth.ComponentCollector, State: agenthealth.EvidenceHealthy, Freshness: agenthealth.FreshnessFresh},
							{Component: agenthealth.ComponentConfiguration, State: agenthealth.EvidenceHealthy, Freshness: agenthealth.FreshnessFresh},
							{Component: agenthealth.ComponentTelemetryDelivery, State: agenthealth.EvidenceUnknown, Freshness: agenthealth.FreshnessUnknown},
						},
					},
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"run(status) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	if gotEndpoint != defaultStatusEndpoint {
		t.Fatalf(
			"status endpoint = %q, want %q",
			gotEndpoint,
			defaultStatusEndpoint,
		)
	}

	want := "" +
		"Agent health: healthy\n" +
		"Collector state: ready\n" +
		"Configuration generations: desired=4 activated=4 applied=4\n" +
		"Pending dependency teardowns: 0\n" +
		"Status scope: Collector/configuration runtime readiness (legacy projection)\n" +
		"agent_runtime: healthy (freshness=fresh)\n" +
		"Evidence assessment: unknown\n" +
		"collector: healthy (freshness=fresh)\n" +
		"configuration: healthy (freshness=fresh)\n" +
		"telemetry_delivery: unknown (freshness=unknown)\n"

	if stdout.String() != want {
		t.Fatalf(
			"status stdout = %q, want %q",
			stdout.String(),
			want,
		)
	}
}

func TestRunStatusReportsUnavailableAgent(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"status"},
		&stdout,
		&stderr,
		dependencies{
			fetchHealthStatus: func(
				context.Context,
				string,
			) (agenthealth.Snapshot, error) {
				return agenthealth.Snapshot{}, errors.New(
					"connect: connection refused",
				)
			},
		},
	)

	if exitCode != 1 {
		t.Fatalf(
			"run(status) exit code = %d, want 1; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	if stdout.Len() != 0 {
		t.Fatalf("status stdout = %q, want empty", stdout.String())
	}

	want := "" +
		"agentctl status: status endpoint is unavailable (request_failed)\n"

	if stderr.String() != want {
		t.Fatalf(
			"status stderr = %q, want %q",
			stderr.String(),
			want,
		)
	}
}

func TestRunStatusRendersLastError(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"status"},
		&stdout,
		&stderr,
		dependencies{
			fetchHealthStatus: func(
				context.Context,
				string,
			) (agenthealth.Snapshot, error) {
				return agenthealth.Snapshot{
					Status:              agenthealth.StatusDegraded,
					CollectorState:      agenthealth.CollectorStateFailed,
					DesiredGeneration:   4,
					ActivatedGeneration: 4,
					AppliedGeneration:   3,
					PendingTeardowns:    1,
					LastError:           "Collector exited unexpectedly",
					CollectorFailures: []agenthealth.CollectorFailure{{
						Receiver: "redis",
						Endpoint: "redis:6379",
						Code:     "connection_failed",
					}},
				}, nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"run(status) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	want := "" +
		"Agent health: degraded\n" +
		"Collector state: failed\n" +
		"Configuration generations: desired=4 activated=4 applied=3\n" +
		"Pending dependency teardowns: 1\n" +
		"Status scope: Collector/configuration runtime readiness (legacy projection)\n" +
		"agent_runtime: unknown (freshness=unknown)\n" +
		"Evidence assessment: unknown\n" +
		"collector: unknown (freshness=unknown)\n" +
		"configuration: unknown (freshness=unknown)\n" +
		"telemetry_delivery: unknown (freshness=unknown)\n" +
		"Last error: Collector exited unexpectedly\n" +
		"Collector endpoint failure: receiver=redis endpoint=redis:6379 code=connection_failed\n"

	if stdout.String() != want {
		t.Fatalf(
			"status stdout = %q, want %q",
			stdout.String(),
			want,
		)
	}
}
