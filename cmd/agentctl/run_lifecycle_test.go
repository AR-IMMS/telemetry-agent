package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestRunCollectorUsesDefaultStatePath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	var gotStatePath string

	exitCode := runCollector(
		context.Background(),
		nil,
		&stdout,
		&stderr,
		dependencies{
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
	if gotStatePath != agentstate.DefaultPath() {
		t.Fatalf(
			"state path = %q, want %q",
			gotStatePath,
			agentstate.DefaultPath(),
		)
	}
	if stdout.String() != "Collector stopped\n" {
		t.Fatalf(
			"stdout = %q, want Collector stopped message",
			stdout.String(),
		)
	}
}

func TestRunCollectorPassesDependencyTeardownToRuntime(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	statePath := "test-state.json"
	teardownCalled := false
	runtimeReceivedTeardown := false

	teardown := agentlifecycle.DependencyTeardownFunc(func(
		ctx context.Context,
		name string,
		action agentstate.TeardownAction,
		resources []agentstate.OwnedResource,
	) error {
		teardownCalled = true

		if name != "node-exporter" {
			t.Fatalf("teardown name = %q, want node-exporter", name)
		}
		if action != agentstate.TeardownActionDisable {
			t.Fatalf("teardown action = %q, want disable", action)
		}

		return nil
	})

	exitCode := runCollector(
		context.Background(),
		[]string{"--state-path", statePath},
		&stdout,
		&stderr,
		dependencies{
			teardownDependency: teardown,
			runCollectorRuntime: func(
				ctx context.Context,
				gotStatePath string,
				gotTeardown agentlifecycle.DependencyTeardownFunc,
			) error {
				if gotStatePath != statePath {
					t.Fatalf(
						"state path = %q, want %q",
						gotStatePath,
						statePath,
					)
				}
				if gotTeardown == nil {
					t.Fatal("runtime teardown = nil, want executor")
				}

				runtimeReceivedTeardown = true

				return gotTeardown(
					ctx,
					"node-exporter",
					agentstate.TeardownActionDisable,
					nil,
				)
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
	if !runtimeReceivedTeardown {
		t.Fatal("runtime did not receive teardown executor")
	}
	if !teardownCalled {
		t.Fatal("runtime teardown executor was not called")
	}
}
