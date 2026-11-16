package agentlifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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

func TestConfigurationCoordinatorRejectsStaleActivationWithoutBlockingUpdates(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	_, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 1

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	activationStarted := make(chan struct{})
	releaseActivation := make(chan struct{})
	activatedGenerations := make([]uint64, 0, 2)

	coordinator := configurationCoordinator{
		store: store,
		activate: func(
			_ context.Context,
			state agentstate.State,
		) error {
			activatedGenerations = append(
				activatedGenerations,
				state.DesiredGeneration,
			)

			if state.DesiredGeneration == 1 {
				close(activationStarted)
				<-releaseActivation
			}

			return nil
		},
	}

	firstApplyDone := make(chan error, 1)

	go func() {
		firstApplyDone <- coordinator.Apply(context.Background())
	}()

	select {
	case <-activationStarted:
	case <-time.After(time.Second):
		t.Fatal("first configuration activation did not start")
	}

	mutationDone := make(chan error, 1)

	go func() {
		_, err := store.Update(func(state *agentstate.State) error {
			state.DesiredGeneration = 2

			return nil
		})
		mutationDone <- err
	}()

	select {
	case err := <-mutationDone:
		if err != nil {
			t.Fatalf("update desired generation during activation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("desired-state mutation blocked during configuration activation")
	}

	close(releaseActivation)

	select {
	case err := <-firstApplyDone:
		if !errors.Is(err, ErrStaleConfiguration) {
			t.Fatalf("first Apply() error = %v, want stale configuration", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first configuration activation did not finish")
	}

	if err := coordinator.Apply(context.Background()); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ActivatedGeneration != 2 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 2",
			state.ActivatedGeneration,
		)
	}

	if got, want := len(activatedGenerations), 2; got != want {
		t.Fatalf("activation count = %d, want %d", got, want)
	}
	if activatedGenerations[0] != 1 || activatedGenerations[1] != 2 {
		t.Fatalf(
			"activated generations = %v, want [1 2]",
			activatedGenerations,
		)
	}
}

func TestConfigurationCoordinatorSkipsAlreadyActivatedGeneration(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)
	if err := store.Save(agentstate.State{
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	activationCalls := 0
	coordinator := configurationCoordinator{
		store: store,
		activate: func(context.Context, agentstate.State) error {
			activationCalls++
			return nil
		},
	}

	if err := coordinator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if activationCalls != 0 {
		t.Fatalf("activation calls = %d, want 0", activationCalls)
	}
}

func TestConfigurationPlanIsAnImmutableSnapshot(t *testing.T) {
	original := agentstate.State{
		DesiredGeneration: 2,
		Dependencies: map[string]agentstate.DependencyState{
			"redis": {
				Enabled: true,
				PendingTeardown: &agentstate.PendingTeardown{
					Action: agentstate.TeardownActionDisable, Generation: 1,
				},
			},
		},
	}
	plan := newConfigurationPlan(original)
	original.Dependencies["redis"] = agentstate.DependencyState{Enabled: false}

	planned := plan.snapshot.Dependencies["redis"]
	if !planned.Enabled || planned.PendingTeardown == nil {
		t.Fatalf("plan snapshot changed with source state: %#v", planned)
	}
	if plan.revision != 2 {
		t.Fatalf("plan revision = %d, want 2", plan.revision)
	}
}
