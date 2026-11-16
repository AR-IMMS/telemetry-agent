package migrations

import (
	"database/sql"
	"testing"

	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
)

func TestBuildAgentRegistryPreservesContiguousContextOwnedVersions(t *testing.T) {
	registry, err := BuildAgentRegistry(Hooks{
		InitializeAgentState: func(*sql.DB) error { return nil },
		ReadAgentState: func(*sql.Tx) (remoteconfigsqlite.AgentState, error) {
			return remoteconfigsqlite.AgentState{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		version int
		name    string
	}{
		{1, "Agent state and activation journal"},
		{2, "Installation identity"},
		{3, "Core Gateway snapshot"},
		{4, "Core Gateway runtime"},
		{5, "Enrollment attempts"},
		{6, "Integration desired state"},
		{7, "Candidate activation mapping"},
		{8, "Candidate attempt history"},
		{9, "Reconciliation diagnostics"},
		{10, "Discovery inventory"},
		{11, "Inventory write revision"},
		{12, "Monitoring proposals"},
		{13, "Integration templates"},
		{14, "Operations execution and outbox"},
	}
	if len(registry) != len(want) {
		t.Fatalf("migration registry length=%d, want %d", len(registry), len(want))
	}
	for index, expected := range want {
		if got := registry[index]; got.Version != expected.version || got.Name != expected.name || got.Apply == nil {
			t.Errorf("migration[%d]=(%d,%q,apply=%t), want (%d,%q,apply=true)", index, got.Version, got.Name, got.Apply != nil, expected.version, expected.name)
		}
	}
}

func TestBuildAgentRegistryRequiresOwnerCallbacks(t *testing.T) {
	_, err := BuildAgentRegistry(Hooks{})
	if err == nil {
		t.Fatal("BuildAgentRegistry() accepted missing context-owned callbacks")
	}
}
