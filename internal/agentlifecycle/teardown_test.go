package agentlifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestTeardownCoordinatorExecutesAppliedOwnedDisable(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {
				Enabled: false,
				Ownership: &agentstate.OwnershipRecord{
					Resources: []agentstate.OwnedResource{
						{
							Kind:       "systemd-service",
							Identifier: "ar-imms-node-exporter.service",
						},
					},
				},
				PendingTeardown: &agentstate.PendingTeardown{
					Action:     agentstate.TeardownActionDisable,
					Generation: 4,
				},
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var gotName string
	var gotAction agentstate.TeardownAction
	var gotResources []agentstate.OwnedResource

	coordinator := teardownCoordinator{
		store: store,
		teardown: func(
			ctx context.Context,
			name string,
			action agentstate.TeardownAction,
			resources []agentstate.OwnedResource,
		) error {
			gotName = name
			gotAction = action
			gotResources = resources

			return nil
		},
	}

	if err := coordinator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if gotName != "node-exporter" {
		t.Fatalf("teardown name = %q, want node-exporter", gotName)
	}
	if gotAction != agentstate.TeardownActionDisable {
		t.Fatalf(
			"teardown action = %q, want disable",
			gotAction,
		)
	}
	if len(gotResources) != 1 ||
		gotResources[0].Identifier != "ar-imms-node-exporter.service" {
		t.Fatalf("teardown resources = %#v", gotResources)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	dependency := state.Dependencies["node-exporter"]

	if dependency.PendingTeardown != nil {
		t.Fatalf(
			"PendingTeardown = %#v, want nil",
			dependency.PendingTeardown,
		)
	}
	if dependency.Enabled {
		t.Fatal("node-exporter Enabled = true, want false")
	}
}

func TestTeardownCoordinatorDoesNotHoldStateLockDuringExternalTeardown(
	t *testing.T,
) {
	store := agentstate.NewFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Save(agentstate.State{
		DesiredGeneration: 1,
		AppliedGeneration: 1,
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {
				Ownership: &agentstate.OwnershipRecord{Resources: []agentstate.OwnedResource{{
					Kind: "service", Identifier: "node-exporter",
				}}},
				PendingTeardown: &agentstate.PendingTeardown{
					Action: agentstate.TeardownActionDisable, Generation: 1,
				},
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	teardownStarted := make(chan struct{})
	releaseTeardown := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseTeardown)
		}
	}()
	applyDone := make(chan error, 1)
	coordinator := teardownCoordinator{
		store: store,
		teardown: func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error {
			close(teardownStarted)
			<-releaseTeardown
			return nil
		},
	}
	go func() { applyDone <- coordinator.Apply(context.Background()) }()
	select {
	case <-teardownStarted:
	case <-time.After(time.Second):
		t.Fatal("dependency teardown did not start")
	}

	updateDone := make(chan error, 1)
	go func() {
		_, err := store.Update(func(state *agentstate.State) error {
			state.Collector.HealthEndpoint = "http://127.0.0.1:13133"
			return nil
		})
		updateDone <- err
	}()
	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("update state during teardown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("state update blocked during external teardown")
	}

	close(releaseTeardown)
	released = true
	if err := <-applyDone; err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

func TestTeardownSerializesWithDependencyReenable(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "otel.yaml")
	store := agentstate.NewFileStore(filepath.Join(root, "state.json"))
	if err := store.Save(agentstate.State{
		Collector:         agentstate.CollectorContext{ConfigPath: configPath},
		DesiredGeneration: 1,
		AppliedGeneration: 1,
		Dependencies: map[string]agentstate.DependencyState{
			"node-exporter": {
				Ownership: &agentstate.OwnershipRecord{Resources: []agentstate.OwnedResource{{
					Kind: "service", Identifier: "node-exporter",
				}}},
				PendingTeardown: &agentstate.PendingTeardown{Action: agentstate.TeardownActionDisable, Generation: 1},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	coordinator := teardownCoordinator{
		store:    store,
		lockPath: configPath + ".lifecycle",
		teardown: func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error {
			close(started)
			<-release
			return nil
		},
	}
	teardownDone := make(chan error, 1)
	go func() { teardownDone <- coordinator.Apply(context.Background()) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("teardown did not start")
	}
	reenableDone := make(chan error, 1)
	go func() {
		lock, err := agentstate.AcquireLifecycleLock(configPath + ".lifecycle")
		if err != nil {
			reenableDone <- err
			return
		}
		_, updateErr := store.Update(func(state *agentstate.State) error {
			_, err := state.EnableDependency("node-exporter")
			return err
		})
		closeErr := lock.Close()
		if updateErr != nil {
			reenableDone <- updateErr
			return
		}
		reenableDone <- closeErr
	}()
	select {
	case err := <-reenableDone:
		t.Fatalf("re-enable completed during destructive teardown: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-teardownDone; err != nil {
		t.Fatal(err)
	}
	if err := <-reenableDone; err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dependencies["node-exporter"].Enabled {
		t.Fatal("dependency not enabled after teardown lock released")
	}
}
