package agentlifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
)

func TestRollbackUnappliedConfigurationRestoresCommittedArtifact(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "otel.yaml")
	if err := os.WriteFile(configPath, []byte("candidate: true\n"), 0o640); err != nil {
		t.Fatalf("write candidate config: %v", err)
	}
	if err := bootstrap.CommitLastKnownGoodConfig(configPath); err != nil {
		t.Fatalf("commit last-known-good config: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("bad-candidate: true\n"), 0o640); err != nil {
		t.Fatalf("write failing candidate config: %v", err)
	}
	store := agentstate.NewFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Save(agentstate.State{
		DesiredGeneration:   2,
		ActivatedGeneration: 2,
		AppliedGeneration:   1,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := rollbackUnappliedConfiguration(context.Background(), store, configPath, 2); err != nil {
		t.Fatalf("rollbackUnappliedConfiguration() error = %v", err)
	}
	gotConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	if string(gotConfig) != "candidate: true\n" {
		t.Fatalf("restored config = %q, want committed candidate", gotConfig)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActivatedGeneration != 1 || state.DesiredGeneration != 2 {
		t.Fatalf("generations after rollback: desired=%d activated=%d", state.DesiredGeneration, state.ActivatedGeneration)
	}
}
