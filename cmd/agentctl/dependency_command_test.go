package main

import (
	"bytes"
	"context"
	"path/filepath"
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
				action agentstate.TeardownAction,
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
				if action != agentstate.TeardownActionDisable {
					t.Fatalf(
						"action = %q, want %q",
						action,
						agentstate.TeardownActionDisable,
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
				action agentstate.TeardownAction,
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
				if action != agentstate.TeardownActionUninstall {
					t.Fatalf(
						"action = %q, want %q",
						action,
						agentstate.TeardownActionUninstall,
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
