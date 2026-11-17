package application

import (
	"testing"

	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

func TestRecordDependencyOwnershipIsCopyOnWrite(t *testing.T) {
	input := OwnershipSnapshot{
		Dependencies: map[string]*servicedomain.OwnershipRecord{
			"node-exporter": nil,
		},
	}
	resources := []servicedomain.OwnedResource{
		{Kind: "systemd-unit", Identifier: "ar-imms-node-exporter.service"},
	}

	got, err := RecordDependencyOwnership(input, " Node-Exporter ", resources)
	if err != nil {
		t.Fatalf("RecordDependencyOwnership() error = %v", err)
	}
	if input.Dependencies["node-exporter"] != nil {
		t.Fatalf("RecordDependencyOwnership() mutated input: %#v", input)
	}
	if !OwnsDependency(got, "NODE-EXPORTER") {
		t.Fatal("OwnsDependency() = false, want true")
	}
	resources[0].Identifier = "mutated"
	if got.Dependencies["node-exporter"].Resources[0].Identifier != "ar-imms-node-exporter.service" {
		t.Fatalf("stored resources alias caller input: %#v", got.Dependencies["node-exporter"])
	}
}

func TestRecordDependencyOwnershipRequiresExistingDependency(t *testing.T) {
	_, err := RecordDependencyOwnership(OwnershipSnapshot{}, "missing", []servicedomain.OwnedResource{
		{Kind: "service", Identifier: "example"},
	})
	if err == nil || err.Error() != `dependency "missing" has no persisted Agent state` {
		t.Fatalf("RecordDependencyOwnership() error = %v", err)
	}
}
