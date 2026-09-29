package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestRunDependencyDisablePassesExplicitStatePathToManagedLifecycle(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	statePath := filepath.Join(t.TempDir(), "state.json")
	lifecycleCalls := 0

	exitCode := runDependency(
		context.Background(),
		[]string{
			"disable",
			"windows-exporter",
			"--state-path",
			statePath,
		},
		&stdout,
		&stderr,
		dependencies{
			manageDependencyLifecycle: func(
				ctx context.Context,
				name string,
				gotStatePath string,
				action dependencyLifecycleAction,
			) error {
				lifecycleCalls++

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
				if action != dependencyLifecycleDisable {
					t.Fatalf(
						"action = %q, want %q",
						action,
						dependencyLifecycleDisable,
					)
				}

				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(disable) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}
	if lifecycleCalls != 1 {
		t.Fatalf(
			"managed lifecycle calls = %d, want 1",
			lifecycleCalls,
		)
	}
	if stdout.String() != "Dependency disable scheduled: windows-exporter\n" {
		t.Fatalf("runDependency(disable) stdout = %q", stdout.String())
	}
}

func TestRunDependencyUninstallPassesExplicitStatePathToManagedLifecycle(
	t *testing.T,
) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	statePath := filepath.Join(t.TempDir(), "state.json")
	lifecycleCalls := 0

	exitCode := runDependency(
		context.Background(),
		[]string{
			"uninstall",
			"windows-exporter",
			"--state-path",
			statePath,
		},
		&stdout,
		&stderr,
		dependencies{
			manageDependencyLifecycle: func(
				ctx context.Context,
				name string,
				gotStatePath string,
				action dependencyLifecycleAction,
			) error {
				lifecycleCalls++

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
				if action != dependencyLifecycleUninstall {
					t.Fatalf(
						"action = %q, want %q",
						action,
						dependencyLifecycleUninstall,
					)
				}

				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(uninstall) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}
	if lifecycleCalls != 1 {
		t.Fatalf(
			"managed lifecycle calls = %d, want 1",
			lifecycleCalls,
		)
	}
	if stdout.String() != "Dependency uninstall scheduled: windows-exporter\n" {
		t.Fatalf("runDependency(uninstall) stdout = %q", stdout.String())
	}
}

func TestRunDependencyPendingListsScheduledTeardowns(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	_, err := store.Update(func(state *agentstate.State) error {
		state.AppliedGeneration = 6
		state.Dependencies = map[string]agentstate.DependencyState{
			"windows-exporter": {
				PendingTeardown: &agentstate.PendingTeardown{
					Action:     agentstate.TeardownActionUninstall,
					Generation: 7,
				},
			},
		}

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{
			"pending",
			"--state-path",
			statePath,
		},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(pending) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	want := "" +
		"Pending dependency teardowns:\n" +
		"- windows-exporter: uninstall, generation 7 " +
		"(applied generation: 6)\n"

	if stdout.String() != want {
		t.Fatalf(
			"runDependency(pending) stdout = %q, want %q",
			stdout.String(),
			want,
		)
	}
}

func TestRunDependencyStatusMarksAbsentDependencyNotInstalled(
	t *testing.T,
) {
	deps := testDependencies(
		identity.PlatformInfo{OS: "windows", Architecture: "amd64"},
	)
	deps.listDependencyStatuses = func(
		context.Context,
	) ([]dependency.StatusResult, error) {
		return []dependency.StatusResult{
			{
				Definition: dependency.Definition{
					Name: "windows-exporter",
				},
				Status: dependency.Status{
					Availability: dependency.AvailabilityDisabled,
					Health:       dependency.HealthUnknown,
				},
			},
		}, nil
	}

	statePath := filepath.Join(t.TempDir(), "state.json")

	code, stdout, stderr := runForTest(
		t,
		[]string{
			"dependency",
			"status",
			"--state-path",
			statePath,
		},
		deps,
	)

	if code != 0 {
		t.Fatalf(
			"run(dependency status) exit code = %d, want 0; stderr = %q",
			code,
			stderr,
		)
	}
	if !strings.Contains(
		stdout,
		"- windows-exporter: not installed",
	) {
		t.Fatalf(
			"status stdout = %q, want not-installed dependency",
			stdout,
		)
	}
}

func TestRunDependencyConfigureAppliesOnlyChangedManagedDependencies(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	_, err := store.Update(func(state *agentstate.State) error {
		state.Dependencies = map[string]agentstate.DependencyState{
			"windows-exporter": {
				Enabled: true,
			},
			"libre-hardware-monitor": {
				Enabled: false,
			},
		}

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	var lifecycleCalls []string
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"configure", "--state-path", statePath},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{
					{
						Name:        "windows-exporter",
						DisplayName: "Windows Exporter",
					},
					{
						Name:        "libre-hardware-monitor",
						DisplayName: "Libre Hardware Monitor",
					},
				}, nil
			},
			configureDependencies: func(
				_ context.Context,
				options []dependencyConfigureOption,
				_ io.Writer,
			) (map[string]bool, error) {
				want := []dependencyConfigureOption{
					{
						Definition: dependency.Definition{
							Name:        "windows-exporter",
							DisplayName: "Windows Exporter",
						},
						Enabled: true,
					},
					{
						Definition: dependency.Definition{
							Name:        "libre-hardware-monitor",
							DisplayName: "Libre Hardware Monitor",
						},
						Enabled: false,
					},
				}
				if !reflect.DeepEqual(options, want) {
					t.Fatalf("configure options = %#v, want %#v", options, want)
				}

				return map[string]bool{
					"windows-exporter":       false,
					"libre-hardware-monitor": true,
				}, nil
			},
			manageDependencyLifecycle: func(
				_ context.Context,
				name string,
				gotStatePath string,
				action dependencyLifecycleAction,
			) error {
				if gotStatePath != statePath {
					t.Fatalf(
						"state path = %q, want %q",
						gotStatePath,
						statePath,
					)
				}

				lifecycleCalls = append(
					lifecycleCalls,
					string(action)+":"+name,
				)

				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(configure) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}

	wantCalls := []string{
		"disable:windows-exporter",
		"enable:libre-hardware-monitor",
	}
	if !reflect.DeepEqual(lifecycleCalls, wantCalls) {
		t.Fatalf(
			"lifecycle calls = %v, want %v",
			lifecycleCalls,
			wantCalls,
		)
	}
}

func TestRunDependencyConfigureCancellationDoesNotManageDependencies(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	_, err := store.Update(func(state *agentstate.State) error {
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

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"configure", "--state-path", statePath},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{
					{Name: "windows-exporter"},
				}, nil
			},
			configureDependencies: func(
				context.Context,
				[]dependencyConfigureOption,
				io.Writer,
			) (map[string]bool, error) {
				return nil, errDependencyConfigurationCancelled
			},
			manageDependencyLifecycle: func(
				context.Context,
				string,
				string,
				dependencyLifecycleAction,
			) error {
				t.Fatal("manageDependencyLifecycle must not be called")

				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(configure) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}
	if stdout.String() != "Dependency configuration cancelled.\n" {
		t.Fatalf(
			"runDependency(configure) stdout = %q",
			stdout.String(),
		)
	}
}

func TestRunDependencyConfigureSkipsUnchangedManagedDependencies(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	_, err := store.Update(func(state *agentstate.State) error {
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

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runDependency(
		context.Background(),
		[]string{"configure", "--state-path", statePath},
		&stdout,
		&stderr,
		dependencies{
			listDependencies: func(
				context.Context,
			) ([]dependency.Definition, error) {
				return []dependency.Definition{
					{Name: "windows-exporter"},
				}, nil
			},
			configureDependencies: func(
				context.Context,
				[]dependencyConfigureOption,
				io.Writer,
			) (map[string]bool, error) {
				return map[string]bool{
					"windows-exporter": true,
				}, nil
			},
			manageDependencyLifecycle: func(
				context.Context,
				string,
				string,
				dependencyLifecycleAction,
			) error {
				t.Fatal(
					"manageDependencyLifecycle must not be called for unchanged dependency",
				)

				return nil
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"runDependency(configure) exit code = %d, want 0; stderr = %q",
			exitCode,
			stderr.String(),
		)
	}
}
