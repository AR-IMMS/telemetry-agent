package dependencyapplication

import (
	"testing"

	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
)

func TestEnableDependencyAdvancesGenerationAndCancelsPendingTeardown(t *testing.T) {
	input := Snapshot{
		DesiredGeneration: 7,
		Dependencies: map[string]dependencydomain.DependencyState{
			"redis": {
				Enabled: false,
				PendingTeardown: &dependencydomain.PendingTeardown{
					Action:     dependencydomain.TeardownActionDisable,
					Generation: 7,
				},
			},
		},
	}

	got, changed, err := EnableDependency(input, " Redis ")
	if err != nil {
		t.Fatalf("EnableDependency() error = %v", err)
	}
	if !changed {
		t.Fatal("EnableDependency() changed = false, want true")
	}
	if got.DesiredGeneration != 8 {
		t.Fatalf("generation = %d, want 8", got.DesiredGeneration)
	}
	if dependency := got.Dependencies["redis"]; !dependency.Enabled || dependency.PendingTeardown != nil {
		t.Fatalf("enabled dependency = %#v, want enabled with no pending teardown", dependency)
	}
	if input.DesiredGeneration != 7 || input.Dependencies["redis"].Enabled || input.Dependencies["redis"].PendingTeardown == nil {
		t.Fatalf("EnableDependency() mutated its input snapshot: %#v", input)
	}
}
