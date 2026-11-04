package agentlifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
)

type failActivatedCommit struct {
	store agentstate.Repository
}

func (s failActivatedCommit) Load() (agentstate.State, error) {
	return s.store.Load()
}

func (s failActivatedCommit) Update(mutate agentstate.StateMutation) (agentstate.State, error) {
	return s.store.Update(func(state *agentstate.State) error {
		if state.PendingActivation == nil {
			return mutate(state)
		}
		candidate := *state
		if err := mutate(&candidate); err != nil {
			return err
		}
		if candidate.ActivatedGeneration > state.ActivatedGeneration {
			return errors.New("simulated activation commit failure")
		}
		*state = candidate
		return nil
	})
}

func TestConfigurationCommitFailureRestoresRecoveryArtifact(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	store := agentstate.NewFileStore(statePath)
	oldConfig := filepath.Join(root, "old.yaml")
	if err := os.WriteFile(oldConfig, []byte("known-good"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(agentstate.State{
		DesiredGeneration: 1,
		Dependencies:      map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	rollbackCalled := false
	coordinator := configurationCoordinator{
		store:    failActivatedCommit{store: store},
		lockPath: filepath.Join(root, "agent.lifecycle"),
		prepare:  func(agentstate.State) (string, error) { return oldConfig, nil },
		activate: func(context.Context, agentstate.State) error { return nil },
		recover: func(path string) error {
			rollbackCalled = path == oldConfig
			return nil
		},
	}
	if err := coordinator.Apply(context.Background()); err == nil {
		t.Fatal("Apply() error = nil, want simulated commit failure")
	}
	if !rollbackCalled {
		t.Fatal("recovery callback was not called for failed commit")
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingActivation != nil {
		t.Fatalf("PendingActivation = %#v, want cleared", state.PendingActivation)
	}
}

func TestRecoverPendingActivationBeforeRuntime(t *testing.T) {
	root := t.TempDir()
	store := agentstate.NewFileStore(filepath.Join(root, "state.json"))
	recoveryPath := filepath.Join(root, "previous.yaml")
	if err := store.Save(agentstate.State{
		DesiredGeneration: 2,
		PendingActivation: &agentstate.PendingActivation{Generation: 2, RecoveryPath: recoveryPath},
		Dependencies:      map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	var restored string
	recoverPending := pendingActivationRecovery{
		store:    store,
		lockPath: filepath.Join(root, "agent.lifecycle"),
		recover:  func(path string) error { restored = path; return nil },
	}
	if err := recoverPending.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if restored != recoveryPath {
		t.Fatalf("restored path = %q, want %q", restored, recoveryPath)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingActivation != nil || state.ActivatedGeneration != 0 {
		t.Fatalf("recovered state = %#v, want pending cleared and old revision retained", state)
	}
}

func TestRecoverInterruptedReadinessRestoresAppliedConfigAndKeepsDesiredState(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "otel.yaml")
	recoveryPath := filepath.Join(root, "previous.yaml")
	if err := os.WriteFile(configPath, []byte("candidate: new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recoveryPath, []byte("candidate: applied\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.CommitLastKnownGoodConfigFrom(configPath, configPath); err != nil {
		t.Fatal(err)
	}
	store := agentstate.NewFileStore(filepath.Join(root, "state.json"))
	if err := store.Save(agentstate.State{
		DesiredGeneration:   2,
		ActivatedGeneration: 2,
		AppliedGeneration:   1,
		PendingActivation:   &agentstate.PendingActivation{Generation: 2, RecoveryPath: recoveryPath},
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	recovery := pendingActivationRecovery{
		store:                store,
		lockPath:             filepath.Join(root, "agent.lifecycle"),
		recover:              func(path string) error { return bootstrap.RestoreConfigArtifact(configPath, path) },
		restoreLastKnownGood: func(path string) error { return bootstrap.CommitLastKnownGoodConfigFrom(configPath, path) },
	}
	if err := recovery.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{configPath, configPath + ".last-known-good"} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "candidate: applied\n" {
			t.Fatalf("recovered %s = %q, %v", path, content, err)
		}
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 2 || state.ActivatedGeneration != 1 || state.AppliedGeneration != 1 || state.PendingActivation != nil {
		t.Fatalf("state after interrupted readiness recovery = %+v", state)
	}
}
