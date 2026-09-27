package agentlifecycle

import (
	"context"
	"path/filepath"
	"testing"

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
