package main

import (
	"bytes"
	"context"
	"path/filepath"
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
