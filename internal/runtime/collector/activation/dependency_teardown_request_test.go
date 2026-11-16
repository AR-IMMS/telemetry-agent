package agentlifecycle

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestRequestAllDependencyUninstallsUsesOneGenerationForAllPersistedDependencies(t *testing.T) {
	store := agentstate.NewFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Save(agentstate.State{
		DesiredGeneration:   7,
		ActivatedGeneration: 7,
		AppliedGeneration:   7,
		Dependencies: map[string]agentstate.DependencyState{
			"windows-exporter":       {Enabled: true},
			"libre-hardware-monitor": {Enabled: true},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	changed, err := RequestAllDependencyUninstalls(context.Background(), store)
	if err != nil {
		t.Fatalf("RequestAllDependencyUninstalls() error = %v", err)
	}
	if got, want := changed, []string{"libre-hardware-monitor", "windows-exporter"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed dependencies = %v, want %v", got, want)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.DesiredGeneration != 8 || state.ActivatedGeneration != 7 || state.AppliedGeneration != 7 {
		t.Fatalf("generation after request: desired=%d activated=%d applied=%d", state.DesiredGeneration, state.ActivatedGeneration, state.AppliedGeneration)
	}
	for _, name := range changed {
		dependency := state.Dependencies[name]
		if dependency.Enabled || dependency.PendingTeardown == nil || dependency.PendingTeardown.Action != agentstate.TeardownActionUninstall || dependency.PendingTeardown.Generation != 8 {
			t.Fatalf("dependency %q teardown state = %+v", name, dependency)
		}
	}
}
