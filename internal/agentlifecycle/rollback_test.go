package agentlifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/identity"
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

func TestRollbackRestoresCollectorConfigAndGatewayEndpointPair(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	configPath := filepath.Join(root, "otel.yaml")
	recoveryPath := filepath.Join(root, "previous.yaml")
	if err := os.WriteFile(configPath, []byte("candidate: core\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recoveryPath, []byte("candidate: bootstrap\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sqliteStore := agentstate.NewSQLiteStore(statePath)
	var store agentstate.Repository = sqliteStore
	if err := sqliteStore.Save(agentstate.State{
		Collector: agentstate.CollectorContext{
			ConfigPath:      configPath,
			GatewayEndpoint: "bootstrap.example:4317",
		},
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   3,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	id, err := identity.ParseInstallationID("11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentstate.NewIdentityStore(statePath).EnsureInstallationID(id); err != nil {
		t.Fatal(err)
	}
	gateway := agentstate.NewCoreGatewayStore(statePath)
	accepted, err := gateway.AcceptGatewaySnapshot(coreconfig.GatewaySnapshot{
		InstallationID: id.String(),
		Revision:       9,
		Endpoint:       "core.example:4317",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *agentstate.State) error {
		if err := state.BeginActivation(accepted.Generation, recoveryPath); err != nil {
			return err
		}
		return state.MarkGenerationActivated(accepted.Generation)
	}); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.CommitLastKnownGoodConfigFrom(configPath, recoveryPath); err != nil {
		t.Fatal(err)
	}
	if err := rollbackUnappliedConfiguration(context.Background(), store, configPath, accepted.Generation); err != nil {
		t.Fatal(err)
	}
	active, err := os.ReadFile(configPath)
	if err != nil || string(active) != "candidate: bootstrap\n" {
		t.Fatalf("restored active config = %q, %v", active, err)
	}
	lastGood, err := os.ReadFile(configPath + ".last-known-good")
	if err != nil || string(lastGood) != "candidate: bootstrap\n" {
		t.Fatalf("restored last-good config = %q, %v", lastGood, err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != accepted.Generation || state.ActivatedGeneration != state.AppliedGeneration || state.PendingActivation != nil {
		t.Fatalf("state after rollback = %+v", state)
	}
	effective, err := gateway.EffectiveGateway()
	if err != nil || effective.Owner != coreconfig.CoreOwner || effective.Revision != 9 || effective.Endpoint != "core.example:4317" {
		t.Fatalf("accepted desired Gateway after rollback = %+v, %v", effective, err)
	}
	runtime, err := gateway.GatewayForGeneration(state.ActivatedGeneration)
	if err != nil || runtime.Endpoint != "bootstrap.example:4317" {
		t.Fatalf("runtime Gateway after rollback = %+v, %v", runtime, err)
	}
}
