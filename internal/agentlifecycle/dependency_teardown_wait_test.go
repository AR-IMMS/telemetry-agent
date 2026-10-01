package agentlifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestWaitForDependenciesRemovedWaitsUntilTeardownsComplete(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   8,
		ActivatedGeneration: 8,
		AppliedGeneration:   8,
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {
				Enabled: false,
				PendingTeardown: &agentstate.PendingTeardown{
					Action:     agentstate.TeardownActionUninstall,
					Generation: 8,
				},
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()

	type waitResult struct {
		state agentstate.State
		err   error
	}

	done := make(chan waitResult, 1)

	go func() {
		state, err := WaitForDependenciesRemoved(
			ctx,
			store,
			time.Millisecond,
		)
		done <- waitResult{
			state: state,
			err:   err,
		}
	}()

	select {
	case result := <-done:
		t.Fatalf(
			"WaitForDependenciesRemoved() returned early: %#v",
			result,
		)
	case <-time.After(20 * time.Millisecond):
	}

	if _, err := store.Update(func(
		state *agentstate.State,
	) error {
		return state.CompleteTeardown("node-exporter", 8)
	}); err != nil {
		t.Fatalf("CompleteTeardown() error = %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf(
				"WaitForDependenciesRemoved() error = %v",
				result.err,
			)
		}
		if len(result.state.Dependencies) != 0 {
			t.Fatalf(
				"remaining dependencies = %#v, want none",
				result.state.Dependencies,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("WaitForDependenciesRemoved() did not finish")
	}
}

func TestWaitForDependenciesRemovedDoesNotTreatDisabledDependencyAsRemoved(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {
				Enabled: false,
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := WaitForDependenciesRemoved(ctx, store, time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"WaitForDependenciesRemoved() error = %v, want deadline exceeded",
			err,
		)
	}
}
