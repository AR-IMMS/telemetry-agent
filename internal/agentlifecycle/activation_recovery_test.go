package agentlifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
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
