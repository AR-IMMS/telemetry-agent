package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
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
