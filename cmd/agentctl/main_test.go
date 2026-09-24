package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func TestBootstrapPassesOrderedLinuxLayersAndValidationOptions(t *testing.T) {
	var gotOptions bootstrap.Options

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

	code, _, stderr := runForTest(t, validBootstrapArguments(), deps)

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
	deps.runBootstrap = func(
		_ context.Context,
		options bootstrap.Options,
		_ bootstrap.Downloader,
		_ bootstrap.CommandRunner,
	) (bootstrap.Result, error) {
		gotOptions = options
		return bootstrap.Result{Artifact: bootstrap.Artifact{Version: "0.160.0"}}, nil
	}

	code, _, stderr := runForTest(t, validBootstrapArguments(), deps)

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
		{name: "unsupported subcommand", args: []string{"status"}},
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

	code, stdout, stderr := runForTest(t, validBootstrapArguments(), deps)

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
	deps := testDependencies(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
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

	code, stdout, stderr := runForTest(t, validBootstrapArguments(), deps)

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

func TestRunInvokesSupervisorWithExplicitRuntimeOptions(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var gotOptions supervisor.Options

	exitCode := run(
		context.Background(),
		[]string{
			"run",
			"--collector-path",
			"C:/agent/bin/otelcol-contrib.exe",
			"--config-path",
			"C:/agent/config/otel.yaml",
			"--gateway-endpoint",
			"gateway.example:4317",
			"--health-endpoint",
			"http://127.0.0.1:13133",
			"--startup-timeout",
			"45s",
			"--shutdown-timeout",
			"12s",
		},
		&stdout,
		&stderr,
		dependencies{
			runSupervisor: func(
				ctx context.Context,
				options supervisor.Options,
			) error {
				gotOptions = options
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

	wantOptions := supervisor.Options{
		BinaryPath:      "C:/agent/bin/otelcol-contrib.exe",
		ConfigPath:      "C:/agent/config/otel.yaml",
		GatewayEndpoint: "gateway.example:4317",
		HealthEndpoint:  "http://127.0.0.1:13133",
		StartupTimeout:  45 * time.Second,
		ShutdownTimeout: 12 * time.Second,
	}

	if !reflect.DeepEqual(gotOptions, wantOptions) {
		t.Fatalf(
			"supervisor options = %#v, want %#v",
			gotOptions,
			wantOptions,
		)
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
