package agentlifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestConfigurationCoordinatorMarksSnapshotGenerationActivated(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	_, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 3

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	activationCalls := 0

	coordinator := configurationCoordinator{
		store: store,
		activate: func(
			ctx context.Context,
			state agentstate.State,
		) error {
			activationCalls++

			if state.DesiredGeneration != 3 {
				t.Fatalf(
					"activation desired generation = %d, want 3",
					state.DesiredGeneration,
				)
			}

			return nil
		},
	}

	if err := coordinator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if activationCalls != 1 {
		t.Fatalf("activation calls = %d, want 1", activationCalls)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActivatedGeneration != 3 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 3",
			state.ActivatedGeneration,
		)
	}
}

func TestConfigurationCoordinatorDoesNotAcknowledgeLaterDesiredGeneration(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	_, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 3

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	coordinator := configurationCoordinator{
		store: store,
		activate: func(
			ctx context.Context,
			state agentstate.State,
		) error {
			_, err := store.Update(func(state *agentstate.State) error {
				state.DesiredGeneration = 4

				return nil
			})

			return err
		},
	}

	err = coordinator.Apply(context.Background())
	if err == nil {
		t.Fatal(
			"Apply() error = nil, want stale desired-generation error",
		)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActivatedGeneration != 0 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 0",
			state.ActivatedGeneration,
		)
	}
}
