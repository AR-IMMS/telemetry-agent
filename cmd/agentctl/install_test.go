package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentinstallation"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestRunInstallStagesCurrentBinaryAndOwnedLayout(t *testing.T) {
	platform := identity.PlatformInfo{
		OS:           "linux",
		Architecture: "amd64",
	}

	statePath := filepath.Join(t.TempDir(), "state.json")
	configRoot := t.TempDir()

	layout := agentinstallation.Layout{
		Platform:    "linux",
		StatePath:   statePath,
		ServiceName: agentinstallation.DefaultAgentServiceName,
	}

	var gotOptions agentinstallation.Options

	deps := dependencies{
		collectPlatform: func() (identity.PlatformInfo, error) {
			return platform, nil
		},
		agentInstallationLayout: func(
			gotPlatform identity.PlatformInfo,
		) (agentinstallation.Layout, error) {
			if gotPlatform != platform {
				t.Fatalf("platform = %#v, want %#v", gotPlatform, platform)
			}

			return layout, nil
		},
		currentExecutable: func() (string, error) {
			return "/tmp/release/agentctl", nil
		},
		installAgent: func(
			_ context.Context,
			options agentinstallation.Options,
		) (agentinstallation.Layout, error) {
			gotOptions = options

			return layout, nil
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{
			"install",
			"--config-root",
			configRoot,
			"--gateway-endpoint",
			"gateway.example:4317",
			"--opamp-server-url",
			"ws://127.0.0.1:4320/v1/opamp",
		},
		&stdout,
		&stderr,
		deps,
	)

	if exitCode != 0 {
		t.Fatalf(
			"run(install) exit code = %d; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	if got, want := gotOptions.Platform, platform; got != want {
		t.Fatalf("platform = %#v, want %#v", got, want)
	}
	if got, want := gotOptions.SourceBinaryPath,
		"/tmp/release/agentctl"; got != want {
		t.Fatalf("source binary = %q, want %q", got, want)
	}
	if got, want := gotOptions.ConfigRoot, configRoot; got != want {
		t.Fatalf("config root = %q, want %q", got, want)
	}
	if got, want := gotOptions.GatewayEndpoint,
		"gateway.example:4317"; got != want {
		t.Fatalf("gateway endpoint = %q, want %q", got, want)
	}
	if got, want := gotOptions.OpAMPServerURL, "ws://127.0.0.1:4320/v1/opamp"; got != want {
		t.Fatalf("OpAMP server URL = %q, want %q", got, want)
	}
	if got, want := gotOptions.HealthEndpoint,
		defaultHealthEndpoint; got != want {
		t.Fatalf("health endpoint = %q, want %q", got, want)
	}

	wantOutput := "" +
		"Agent installation complete: ar-imms-telemetry-agent\n" +
		"Agent state path: " + statePath + "\n"

	if got := stdout.String(); got != wantOutput {
		t.Fatalf("stdout = %q, want %q", got, wantOutput)
	}
}

func TestRunInstallAcceptsExplicitLegacyRuntimeRollback(t *testing.T) {
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	layout := agentinstallation.Layout{
		Platform:    "linux",
		StatePath:   filepath.Join(t.TempDir(), "state.db"),
		ServiceName: agentinstallation.DefaultAgentServiceName,
	}
	installCalled := false
	var installedOptions agentinstallation.Options
	deps := dependencies{
		collectPlatform: func() (identity.PlatformInfo, error) {
			return platform, nil
		},
		agentInstallationLayout: func(identity.PlatformInfo) (agentinstallation.Layout, error) {
			return layout, nil
		},
		currentExecutable: func() (string, error) { return "/tmp/agentctl", nil },
		installAgent: func(_ context.Context, options agentinstallation.Options) (agentinstallation.Layout, error) {
			installCalled = true
			installedOptions = options
			return layout, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"install",
		"--config-root", t.TempDir(),
		"--disable-opamp-supervisor",
	}, &stdout, &stderr, deps)
	if code != 0 {
		t.Fatalf("explicit legacy rollback install exit code = %d; stderr=%q", code, stderr.String())
	}
	if !installCalled {
		t.Fatal("explicit legacy rollback did not reach the installer")
	}
	if !installedOptions.DisableSupervisor || installedOptions.OpAMPServerURL != "" {
		t.Fatalf("installer options = %#v, want explicit legacy ownership", installedOptions)
	}
}

func TestRunInstallRejectsConflictingSupervisorOwnershipOptions(t *testing.T) {
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	layout := agentinstallation.Layout{
		Platform: "linux", StatePath: filepath.Join(t.TempDir(), "state.db"),
		ServiceName: agentinstallation.DefaultAgentServiceName,
	}
	installCalled := false
	deps := dependencies{
		collectPlatform:         func() (identity.PlatformInfo, error) { return platform, nil },
		agentInstallationLayout: func(identity.PlatformInfo) (agentinstallation.Layout, error) { return layout, nil },
		currentExecutable:       func() (string, error) { return "/tmp/agentctl", nil },
		installAgent: func(_ context.Context, _ agentinstallation.Options) (agentinstallation.Layout, error) {
			installCalled = true
			return layout, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"install", "--config-root", t.TempDir(),
		"--opamp-server-url", "ws://127.0.0.1:4320/v1/opamp",
		"--disable-opamp-supervisor",
	}, &stdout, &stderr, deps)
	if code != 2 || !strings.Contains(stderr.String(), "cannot be combined") {
		t.Fatalf("conflicting ownership options exit=%d stderr=%q", code, stderr.String())
	}
	if installCalled {
		t.Fatal("conflicting ownership options reached the installer")
	}
}
