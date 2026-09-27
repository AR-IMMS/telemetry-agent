package agentstate

import (
	"reflect"
	"testing"
)

func TestStateEnableDependencyMarksDependencyEnabledAndAdvancesGeneration(
	t *testing.T,
) {
	state := State{
		DesiredGeneration: 3,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: false,
			},
		},
	}

	changed, err := state.EnableDependency("  NODE-EXPORTER  ")
	if err != nil {
		t.Fatalf("EnableDependency() error = %v", err)
	}
	if !changed {
		t.Fatal("EnableDependency() changed = false, want true")
	}
	if !state.Dependencies["node-exporter"].Enabled {
		t.Fatal("node-exporter Enabled = false, want true")
	}
	if state.DesiredGeneration != 4 {
		t.Fatalf(
			"DesiredGeneration = %d, want 4",
			state.DesiredGeneration,
		)
	}
}

func TestStateEnableDependencyDoesNotAdvanceGenerationWhenAlreadyEnabled(
	t *testing.T,
) {
	state := State{
		DesiredGeneration: 3,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: true,
			},
		},
	}

	changed, err := state.EnableDependency("node-exporter")
	if err != nil {
		t.Fatalf("EnableDependency() error = %v", err)
	}
	if changed {
		t.Fatal("EnableDependency() changed = true, want false")
	}
	if state.DesiredGeneration != 3 {
		t.Fatalf(
			"DesiredGeneration = %d, want 3",
			state.DesiredGeneration,
		)
	}
}

func TestStateDisableDependencyMarksDependencyDisabledAndAdvancesGeneration(
	t *testing.T,
) {
	state := State{
		DesiredGeneration: 7,
		Dependencies: map[string]DependencyState{
			"windows-exporter": {
				Enabled: true,
			},
		},
	}

	changed, err := state.DisableDependency(" WINDOWS-EXPORTER ")
	if err != nil {
		t.Fatalf("DisableDependency() error = %v", err)
	}
	if !changed {
		t.Fatal("DisableDependency() changed = false, want true")
	}
	if state.Dependencies["windows-exporter"].Enabled {
		t.Fatal("windows-exporter Enabled = true, want false")
	}
	if state.DesiredGeneration != 8 {
		t.Fatalf(
			"DesiredGeneration = %d, want 8",
			state.DesiredGeneration,
		)
	}
}

func TestStateMarkGenerationAppliedAcknowledgesCurrentActivatedGeneration(
	t *testing.T,
) {
	state := State{
		DesiredGeneration:   8,
		ActivatedGeneration: 8,
		AppliedGeneration:   7,
	}

	if err := state.MarkGenerationApplied(8); err != nil {
		t.Fatalf("MarkGenerationApplied() error = %v", err)
	}
	if state.AppliedGeneration != 8 {
		t.Fatalf(
			"AppliedGeneration = %d, want 8",
			state.AppliedGeneration,
		)
	}
}

func TestStateDisableDependencySchedulesTeardownForNewGeneration(
	t *testing.T,
) {
	state := State{
		DesiredGeneration: 10,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: true,
			},
		},
	}

	_, err := state.DisableDependency("node-exporter")
	if err != nil {
		t.Fatalf("DisableDependency() error = %v", err)
	}

	pending := state.Dependencies["node-exporter"].PendingTeardown
	if pending == nil {
		t.Fatal("PendingTeardown = nil, want disable teardown")
	}
	if pending.Action != TeardownActionDisable {
		t.Fatalf(
			"PendingTeardown Action = %q, want %q",
			pending.Action,
			TeardownActionDisable,
		)
	}
	if pending.Generation != 11 {
		t.Fatalf(
			"PendingTeardown Generation = %d, want 11",
			pending.Generation,
		)
	}
}

func TestStateRequestUninstallDisablesDependencyAndSchedulesUninstall(
	t *testing.T,
) {
	state := State{
		DesiredGeneration: 12,
		Dependencies: map[string]DependencyState{
			"libre-hardware-monitor": {
				Enabled: true,
			},
		},
	}

	changed, err := state.RequestUninstall("libre-hardware-monitor")
	if err != nil {
		t.Fatalf("RequestUninstall() error = %v", err)
	}
	if !changed {
		t.Fatal("RequestUninstall() changed = false, want true")
	}

	dependency := state.Dependencies["libre-hardware-monitor"]
	if dependency.Enabled {
		t.Fatal("Libre Hardware Monitor Enabled = true, want false")
	}
	if state.DesiredGeneration != 13 {
		t.Fatalf(
			"DesiredGeneration = %d, want 13",
			state.DesiredGeneration,
		)
	}
	if dependency.PendingTeardown == nil {
		t.Fatal("PendingTeardown = nil, want uninstall teardown")
	}
	if dependency.PendingTeardown.Action != TeardownActionUninstall {
		t.Fatalf(
			"PendingTeardown Action = %q, want %q",
			dependency.PendingTeardown.Action,
			TeardownActionUninstall,
		)
	}
	if dependency.PendingTeardown.Generation != 13 {
		t.Fatalf(
			"PendingTeardown Generation = %d, want 13",
			dependency.PendingTeardown.Generation,
		)
	}
}

func TestStateCompleteUninstallRemovesDependencyAfterAcknowledgement(
	t *testing.T,
) {
	state := State{
		DesiredGeneration: 4,
		AppliedGeneration: 4,
		Dependencies: map[string]DependencyState{
			"windows-exporter": {
				Enabled: false,
				PendingTeardown: &PendingTeardown{
					Action:     TeardownActionUninstall,
					Generation: 4,
				},
			},
		},
	}

	if err := state.CompleteTeardown("windows-exporter", 4); err != nil {
		t.Fatalf("CompleteTeardown() error = %v", err)
	}

	if _, exists := state.Dependencies["windows-exporter"]; exists {
		t.Fatal("windows-exporter state still exists after uninstall teardown")
	}
}

func TestStateRecordsOwnedResourcesWithoutChangingGeneration(
	t *testing.T,
) {
	state := State{
		DesiredGeneration: 9,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: true,
			},
		},
	}

	resources := []OwnedResource{
		{
			Kind:       "systemd-unit",
			Identifier: "/etc/systemd/system/ar-imms-node-exporter.service",
		},
		{
			Kind:       "directory",
			Identifier: "/opt/ar-imms/node-exporter",
		},
	}

	if err := state.RecordOwnership("node-exporter", resources); err != nil {
		t.Fatalf("RecordOwnership() error = %v", err)
	}

	ownership := state.Dependencies["node-exporter"].Ownership
	if ownership == nil {
		t.Fatal("Ownership = nil, want recorded ownership")
	}
	if !reflect.DeepEqual(ownership.Resources, resources) {
		t.Fatalf(
			"Ownership Resources = %#v, want %#v",
			ownership.Resources,
			resources,
		)
	}
	if state.DesiredGeneration != 9 {
		t.Fatalf(
			"DesiredGeneration = %d, want 9",
			state.DesiredGeneration,
		)
	}
}

func TestStateReportsDependencyOwnedOnlyWithRecordedResources(
	t *testing.T,
) {
	state := State{
		Dependencies: map[string]DependencyState{
			"windows-exporter": {
				Ownership: &OwnershipRecord{
					Resources: []OwnedResource{
						{
							Kind:       "windows-service",
							Identifier: "windows_exporter",
						},
					},
				},
			},
			"node-exporter": {},
		},
	}

	if !state.OwnsDependency(" WINDOWS-EXPORTER ") {
		t.Fatal("OwnsDependency(windows-exporter) = false, want true")
	}
	if state.OwnsDependency("node-exporter") {
		t.Fatal("OwnsDependency(node-exporter) = true, want false")
	}
}

func TestFileStoreUpdatePersistsOneStateMutation(t *testing.T) {
	store := NewFileStore(t.TempDir() + "/state.json")

	if err := store.Save(State{
		DesiredGeneration: 2,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: true,
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Update(func(state *State) error {
		_, err := state.DisableDependency("node-exporter")

		return err
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if got.DesiredGeneration != 3 {
		t.Fatalf(
			"updated DesiredGeneration = %d, want 3",
			got.DesiredGeneration,
		)
	}
	if got.Dependencies["node-exporter"].Enabled {
		t.Fatal("updated node-exporter Enabled = true, want false")
	}

	persisted, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(persisted, got) {
		t.Fatalf("persisted state = %#v, want %#v", persisted, got)
	}
}

func TestStateDoesNotRequireApplyUntilDesiredGenerationIsActivated(
	t *testing.T,
) {
	state := State{
		DesiredGeneration:   5,
		ActivatedGeneration: 4,
		AppliedGeneration:   4,
	}

	if state.RequiresApply() {
		t.Fatal("RequiresApply() = true, want false")
	}
}

func TestStateMarkGenerationAppliedAcceptsActivatedGenerationBeforeLaterDesiredState(
	t *testing.T,
) {
	state := State{
		DesiredGeneration:   9,
		ActivatedGeneration: 8,
		AppliedGeneration:   7,
	}

	if err := state.MarkGenerationApplied(8); err != nil {
		t.Fatalf("MarkGenerationApplied() error = %v", err)
	}
	if state.AppliedGeneration != 8 {
		t.Fatalf(
			"AppliedGeneration = %d, want 8",
			state.AppliedGeneration,
		)
	}
}

func TestStateMarkGenerationActivatedAcknowledgesCurrentDesiredGeneration(
	t *testing.T,
) {
	state := State{
		DesiredGeneration:   6,
		ActivatedGeneration: 5,
		AppliedGeneration:   5,
	}

	if err := state.MarkGenerationActivated(6); err != nil {
		t.Fatalf("MarkGenerationActivated() error = %v", err)
	}
	if state.ActivatedGeneration != 6 {
		t.Fatalf(
			"ActivatedGeneration = %d, want 6",
			state.ActivatedGeneration,
		)
	}
}

func TestStateEnableDependencyCancelsPendingTeardown(t *testing.T) {
	state := State{
		DesiredGeneration: 7,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: false,
				PendingTeardown: &PendingTeardown{
					Action:     TeardownActionDisable,
					Generation: 7,
				},
			},
		},
	}

	changed, err := state.EnableDependency("node-exporter")

	if err != nil {
		t.Fatalf("EnableDependency() error = %v", err)
	}
	if !changed {
		t.Fatal("EnableDependency() changed = false, want true")
	}

	dependency := state.Dependencies["node-exporter"]

	if !dependency.Enabled {
		t.Fatal("node-exporter Enabled = false, want true")
	}
	if dependency.PendingTeardown != nil {
		t.Fatalf(
			"PendingTeardown = %#v, want nil after re-enable",
			dependency.PendingTeardown,
		)
	}
	if state.DesiredGeneration != 8 {
		t.Fatalf(
			"DesiredGeneration = %d, want 8",
			state.DesiredGeneration,
		)
	}
}

func TestRequestUninstallFromDisabledDependencyAdvancesGeneration(
	t *testing.T,
) {
	state := State{
		DesiredGeneration:   6,
		ActivatedGeneration: 6,
		AppliedGeneration:   6,
		Dependencies: map[string]DependencyState{
			"windows-exporter": {
				Enabled: false,
			},
		},
	}

	changed, err := state.RequestUninstall("windows-exporter")
	if err != nil {
		t.Fatalf("RequestUninstall() error = %v", err)
	}
	if !changed {
		t.Fatal("RequestUninstall() changed = false, want true")
	}
	if state.DesiredGeneration != 7 {
		t.Fatalf(
			"DesiredGeneration = %d, want 7",
			state.DesiredGeneration,
		)
	}

	dependency := state.Dependencies["windows-exporter"]
	if dependency.PendingTeardown == nil {
		t.Fatal("PendingTeardown = nil, want uninstall teardown")
	}
	if dependency.PendingTeardown.Action != TeardownActionUninstall {
		t.Fatalf(
			"PendingTeardown.Action = %q, want %q",
			dependency.PendingTeardown.Action,
			TeardownActionUninstall,
		)
	}
	if dependency.PendingTeardown.Generation != 7 {
		t.Fatalf(
			"PendingTeardown.Generation = %d, want 7",
			dependency.PendingTeardown.Generation,
		)
	}
}
