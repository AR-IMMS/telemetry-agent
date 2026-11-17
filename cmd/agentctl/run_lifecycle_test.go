package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
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
		action dependencydomain.TeardownAction,
		resources []servicedomain.OwnedResource,
	) error {
		teardownCalled = true

		if name != "node-exporter" {
			t.Fatalf("teardown name = %q, want node-exporter", name)
		}
		if action != dependencydomain.TeardownActionDisable {
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
			prepareInstallationIdentity: skipInstallationIdentityPreparation,
			teardownDependency:          teardown,
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
					dependencydomain.TeardownActionDisable,
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
