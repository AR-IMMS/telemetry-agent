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

func TestValidateAgentInstallationNormalizesAndCopiesOwnership(t *testing.T) {
	installation := servicedomain.AgentInstallation{
		Platform:    " Windows ",
		ServiceName: " agent-service ",
		BinaryPath:  " C:/agent/agent.exe ",
		Ownership: servicedomain.OwnershipRecord{Resources: []servicedomain.OwnedResource{
			{Kind: " file ", Identifier: " C:/agent/agent.exe "},
		}},
	}

	got, err := ValidateAgentInstallation(installation)
	if err != nil {
		t.Fatalf("ValidateAgentInstallation() error = %v", err)
	}
	if got.Platform != "windows" || got.ServiceName != "agent-service" || got.BinaryPath != installation.BinaryPath {
		t.Fatalf("normalized installation = %#v", got)
	}
	if got.Ownership.Resources[0] != (servicedomain.OwnedResource{Kind: "file", Identifier: "C:/agent/agent.exe"}) {
		t.Fatalf("normalized ownership = %#v", got.Ownership)
	}
	installation.Ownership.Resources[0].Identifier = "changed"
	if got.Ownership.Resources[0].Identifier != "C:/agent/agent.exe" {
		t.Fatalf("validated installation aliases caller input: %#v", got)
	}
}

func TestValidateAgentInstallationRejectsMissingOwnership(t *testing.T) {
	_, err := ValidateAgentInstallation(servicedomain.AgentInstallation{
		Platform: "linux", ServiceName: "agent", Ownership: servicedomain.OwnershipRecord{},
	})
	if err == nil || err.Error() != "Agent installation ownership is required" {
		t.Fatalf("ValidateAgentInstallation() error = %v", err)
	}
}
