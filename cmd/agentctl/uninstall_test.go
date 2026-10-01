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

	"github.com/ar-imms/telemetry-agent/internal/agentinstallation"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestRunUninstallPassesStatePathAndTimeoutToAgentUninstaller(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	timeout := 45 * time.Second

	called := false

	deps := dependencies{
		uninstallAgent: func(
			_ context.Context,
			gotStatePath string,
			gotTimeout time.Duration,
		) error {
			called = true

			if gotStatePath != statePath {
				t.Fatalf(
					"state path = %q, want %q",
					gotStatePath,
					statePath,
				)
			}
			if gotTimeout != timeout {
				t.Fatalf(
					"timeout = %s, want %s",
					gotTimeout,
					timeout,
				)
			}

			return nil
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{
			"uninstall",
			"--state-path",
			statePath,
			"--timeout",
			timeout.String(),
		},
		&stdout,
		&stderr,
		deps,
	)

	if exitCode != 0 {
		t.Fatalf(
			"run(uninstall) exit code = %d; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}
	if !called {
		t.Fatal("Agent uninstaller was not called")
	}
	if got, want := stdout.String(),
		"Agent uninstall complete\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestManagedAgentUninstallerRemovesAgentAfterDependenciesAreGone(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	installation := agentstate.AgentInstallation{
		Platform:    "linux",
		ServiceName: "ar-imms-telemetry-agent",
		BinaryPath:  "/opt/ar-imms/telemetry-agent/agentctl",
	}

	store := agentstate.NewFileStore(statePath)
	if err := store.Save(agentstate.State{
		Installation: &installation,
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {
				Enabled: false,
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var steps []string

	uninstaller := managedAgentUninstaller{
		collectPlatform: func() (identity.PlatformInfo, error) {
			steps = append(steps, "collect-platform")

			return identity.PlatformInfo{OS: "linux"}, nil
		},
		currentExecutable: func() (string, error) {
			steps = append(steps, "current-executable")

			return "/tmp/release/agentctl", nil
		},
		resolveExecutable: func(path string) (string, error) {
			steps = append(steps, "resolve-executable")

			return path, nil
		},
		requestUninstalls: func(
			_ context.Context,
			gotStatePath string,
		) error {
			steps = append(steps, "request-uninstalls")

			if gotStatePath != statePath {
				t.Fatalf(
					"request state path = %q, want %q",
					gotStatePath,
					statePath,
				)
			}

			return nil
		},
		waitForDependencies: func(
			_ context.Context,
			gotStatePath string,
		) (agentstate.State, error) {
			steps = append(steps, "wait-for-dependencies")

			if gotStatePath != statePath {
				t.Fatalf(
					"wait state path = %q, want %q",
					gotStatePath,
					statePath,
				)
			}

			return agentstate.State{
				Installation: &installation,
				Dependencies: map[string]agentstate.DependencyState{},
			}, nil
		},
		newRemover: func(
			platform identity.PlatformInfo,
		) (agentinstallation.Remover, error) {
			steps = append(steps, "new-remover")

			if platform.OS != "linux" {
				t.Fatalf("remover platform = %q, want linux", platform.OS)
			}

			return agentRemovalFunc(func(
				_ context.Context,
				gotInstallation agentstate.AgentInstallation,
				gotStatePath string,
			) error {
				steps = append(steps, "remove-agent")

				if !reflect.DeepEqual(gotInstallation, installation) {
					t.Fatalf(
						"installation = %#v, want %#v",
						gotInstallation,
						installation,
					)
				}
				if gotStatePath != statePath {
					t.Fatalf(
						"remove state path = %q, want %q",
						gotStatePath,
						statePath,
					)
				}

				return nil
			}), nil
		},
	}

	if err := uninstaller.Uninstall(
		context.Background(),
		statePath,
		time.Minute,
	); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if got, want := steps, []string{
		"current-executable",
		"resolve-executable",
		"collect-platform",
		"request-uninstalls",
		"wait-for-dependencies",
		"new-remover",
		"remove-agent",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

type agentRemovalFunc func(
	context.Context,
	agentstate.AgentInstallation,
	string,
) error

func (f agentRemovalFunc) Remove(
	ctx context.Context,
	installation agentstate.AgentInstallation,
	statePath string,
) error {
	return f(ctx, installation, statePath)
}

func TestManagedAgentUninstallerRejectsInstalledBinaryBeforeRequestingTeardown(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	installedBinary := "/opt/ar-imms/telemetry-agent/agentctl"

	store := agentstate.NewFileStore(statePath)
	if err := store.Save(agentstate.State{
		Installation: &agentstate.AgentInstallation{
			Platform:   "linux",
			BinaryPath: installedBinary,
		},
		Dependencies: map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	requested := false
	waited := false

	uninstaller := managedAgentUninstaller{
		currentExecutable: func() (string, error) {
			return installedBinary, nil
		},
		resolveExecutable: func(path string) (string, error) {
			return path, nil
		},
		requestUninstalls: func(
			context.Context,
			string,
		) error {
			requested = true

			return nil
		},
		waitForDependencies: func(
			context.Context,
			string,
		) (agentstate.State, error) {
			waited = true

			return agentstate.State{}, nil
		},
	}

	err := uninstaller.Uninstall(
		context.Background(),
		statePath,
		time.Minute,
	)

	if !errors.Is(err, agentinstallation.ErrInstalledAgentCannotUninstall) {
		t.Fatalf(
			"Uninstall() error = %v, want installed Agent rejection",
			err,
		)
	}
	if requested {
		t.Fatal("dependency uninstall request ran after installed-binary rejection")
	}
	if waited {
		t.Fatal("dependency teardown wait ran after installed-binary rejection")
	}
}

func TestDefaultDependenciesWiresAgentUninstaller(t *testing.T) {
	deps := defaultDependencies()

	if deps.uninstallAgent == nil {
		t.Fatal("default Agent uninstaller = nil")
	}
}

func TestManagedAgentUninstallerDoesNotRemoveAgentWhenTeardownWaitFails(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	waitError := errors.New("Collector did not apply teardown generation")

	store := agentstate.NewFileStore(statePath)
	if err := store.Save(agentstate.State{
		Installation: &agentstate.AgentInstallation{
			Platform:   "linux",
			BinaryPath: "/opt/ar-imms/telemetry-agent/agentctl",
		},
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {Enabled: false},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	removerResolved := false

	uninstaller := managedAgentUninstaller{
		collectPlatform: func() (identity.PlatformInfo, error) {
			return identity.PlatformInfo{OS: "linux"}, nil
		},
		currentExecutable: func() (string, error) {
			return "/tmp/release/agentctl", nil
		},
		resolveExecutable: func(path string) (string, error) {
			return path, nil
		},
		requestUninstalls: func(context.Context, string) error {
			return nil
		},
		waitForDependencies: func(
			context.Context,
			string,
		) (agentstate.State, error) {
			return agentstate.State{}, waitError
		},
		newRemover: func(
			identity.PlatformInfo,
		) (agentinstallation.Remover, error) {
			removerResolved = true

			return nil, nil
		},
	}

	err := uninstaller.Uninstall(
		context.Background(),
		statePath,
		time.Minute,
	)

	if !errors.Is(err, waitError) {
		t.Fatalf(
			"Uninstall() error = %v, want teardown wait error",
			err,
		)
	}
	if removerResolved {
		t.Fatal("Agent remover was resolved after teardown wait failure")
	}
}

func TestRunUninstallRejectsNonPositiveTimeout(t *testing.T) {
	called := false

	deps := dependencies{
		uninstallAgent: func(
			context.Context,
			string,
			time.Duration,
		) error {
			called = true

			return nil
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"uninstall", "--timeout", "0s"},
		&stdout,
		&stderr,
		deps,
	)

	if exitCode != 2 {
		t.Fatalf("run(uninstall) exit code = %d, want 2", exitCode)
	}
	if called {
		t.Fatal("Agent uninstaller was called for invalid timeout")
	}
	if !strings.Contains(
		stderr.String(),
		"--timeout must be greater than zero",
	) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
