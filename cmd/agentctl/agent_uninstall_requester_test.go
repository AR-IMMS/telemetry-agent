package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	collectordomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/domain"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func saveTestState(t *testing.T, store agentstate.Repository, state agentstate.State) {
	t.Helper()
	if _, err := store.Update(func(current *agentstate.State) error {
		*current = state
		return nil
	}); err != nil {
		t.Fatalf("persist test state: %v", err)
	}
}

func TestManagedAgentUninstallRequesterUsesPersistedStateWithoutRuntimeRebuild(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := sqlitefixture.NewStore(statePath)
	saveTestState(t, store, agentstate.State{
		Collector: collectordomain.CollectorContext{
			ConfigRoot:      "/srv/ar-imms/config",
			BinaryPath:      "/opt/ar-imms/collector/otelcol-contrib",
			ConfigPath:      "/var/lib/ar-imms/telemetry-agent/collector.yaml",
			GatewayEndpoint: "gateway.example:4317",
		},
		Dependencies: map[string]dependencydomain.DependencyState{
			"node-exporter": {Enabled: true},
		},
	})

	called := false
	requester := managedAgentUninstallRequester{
		requestAll: func(_ context.Context, gotStore agentstate.Repository) ([]string, error) {
			called = true
			state, err := gotStore.Load()
			if err != nil {
				t.Fatalf("request store Load() error = %v", err)
			}
			if state.Collector.ConfigRoot != "/srv/ar-imms/config" || state.Collector.BinaryPath != "/opt/ar-imms/collector/otelcol-contrib" {
				t.Fatalf("request store lost persisted Collector context: %+v", state.Collector)
			}
			if _, exists := state.Dependencies["node-exporter"]; !exists {
				t.Fatal("request store did not contain persisted dependency")
			}
			return []string{"node-exporter"}, nil
		},
	}

	if err := requester.Request(context.Background(), statePath); err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if !called {
		t.Fatal("batch dependency uninstall request was not called")
	}
}
