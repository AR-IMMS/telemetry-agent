package main

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestPrepareAgentInstallationIdentityReusesPersistentID(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.db")
	store := agentstate.NewStore(statePath)
	if _, err := store.Update(func(state *agentstate.State) error {
		state.Collector = agentstate.CollectorContext{
			BinaryPath:      "otelcol",
			ConfigPath:      "otel.yaml",
			GatewayEndpoint: "127.0.0.1:4317",
			HealthEndpoint:  "http://127.0.0.1:13133",
		}
		return nil
	}); err != nil {
		t.Fatalf("save operational state: %v", err)
	}

	if err := prepareAgentInstallationIdentity(statePath); err != nil {
		t.Fatalf("first identity preparation: %v", err)
	}
	first, err := agentstate.NewIdentityStore(statePath).EnsureInstallationID(
		mustIdentityID(t, "11111111-2222-4333-8444-555555555555"),
	)
	if err != nil {
		t.Fatalf("read committed identity: %v", err)
	}
	if err := prepareAgentInstallationIdentity(statePath); err != nil {
		t.Fatalf("restart identity preparation: %v", err)
	}
	second, err := agentstate.NewIdentityStore(statePath).EnsureInstallationID(
		mustIdentityID(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"),
	)
	if err != nil {
		t.Fatalf("read identity after restart: %v", err)
	}
	if second != first {
		t.Fatalf("identity after restart = %q, want %q", second, first)
	}
}

func TestRunCollectorRuntimeRejectsMissingOperationalState(t *testing.T) {
	listenerStarted := false
	runtimeStarted := false
	err := runCollectorRuntime(
		context.Background(),
		filepath.Join(t.TempDir(), "missing-state.db"),
		dependencies{
			prepareInstallationIdentity: prepareAgentInstallationIdentity,
			listenStatus: func(_, _ string) (net.Listener, error) {
				listenerStarted = true
				return stubStatusListener{}, nil
			},
			runCollectorRuntimeWithHealth: func(
				context.Context,
				string,
				agentlifecycle.DependencyTeardownFunc,
				agenthealth.ObservationReporter,
			) error {
				runtimeStarted = true
				return nil
			},
		},
	)
	if err == nil {
		t.Fatal("runCollectorRuntime() error = nil, want missing state error")
	}
	if listenerStarted || runtimeStarted {
		t.Fatalf("startup continued without operational state: listener=%t runtime=%t", listenerStarted, runtimeStarted)
	}
}

func TestRunCollectorRuntimePreparesIdentityBeforeListeningAndStarting(t *testing.T) {
	identityPrepared := false
	runtimeStarted := false
	err := runCollectorRuntime(
		context.Background(),
		"test-state.db",
		dependencies{
			prepareInstallationIdentity: func(path string) error {
				if path != "test-state.db" {
					t.Fatalf("identity state path = %q, want test-state.db", path)
				}
				identityPrepared = true
				return nil
			},
			listenStatus: func(_, _ string) (net.Listener, error) {
				if !identityPrepared {
					t.Fatal("status listener started before identity preparation")
				}
				return stubStatusListener{}, nil
			},
			serveStatus: func(
				context.Context,
				net.Listener,
				agenthealth.Provider,
			) error {
				return nil
			},
			runCollectorRuntimeWithHealth: func(
				context.Context,
				string,
				agentlifecycle.DependencyTeardownFunc,
				agenthealth.ObservationReporter,
			) error {
				if !identityPrepared {
					t.Fatal("Collector runtime started before identity preparation")
				}
				runtimeStarted = true
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("runCollectorRuntime() error = %v", err)
	}
	if !runtimeStarted {
		t.Fatal("Collector runtime did not start")
	}
}

func TestRunCollectorRuntimeStopsWhenIdentityPreparationFails(t *testing.T) {
	wantErr := errors.New("invalid installation identity")
	listenerStarted := false
	runtimeStarted := false
	err := runCollectorRuntime(
		context.Background(),
		"test-state.db",
		dependencies{
			prepareInstallationIdentity: func(string) error {
				return wantErr
			},
			listenStatus: func(_, _ string) (net.Listener, error) {
				listenerStarted = true
				return stubStatusListener{}, nil
			},
			runCollectorRuntimeWithHealth: func(
				context.Context,
				string,
				agentlifecycle.DependencyTeardownFunc,
				agenthealth.ObservationReporter,
			) error {
				runtimeStarted = true
				return nil
			},
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runCollectorRuntime() error = %v, want %v", err, wantErr)
	}
	if listenerStarted || runtimeStarted {
		t.Fatalf("startup continued after identity failure: listener=%t runtime=%t", listenerStarted, runtimeStarted)
	}
}

type stubStatusListener struct{}

func (stubStatusListener) Accept() (net.Conn, error) {
	return nil, errors.New("accept is not used in this test")
}

func (stubStatusListener) Close() error { return nil }

func (stubStatusListener) Addr() net.Addr { return &net.TCPAddr{} }

func mustIdentityID(t *testing.T, value string) identity.InstallationID {
	t.Helper()
	id, err := identity.ParseInstallationID(value)
	if err != nil {
		t.Fatalf("ParseInstallationID(%q) error = %v", value, err)
	}
	return id
}
