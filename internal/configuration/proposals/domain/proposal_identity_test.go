package domain

import "testing"

func TestStableProposalIDIsDeterministicForStableIdentity(t *testing.T) {
	identity := ProposalIdentity{
		Provider: "docker", ProviderInstance: "local", ResourceID: "container-123", IntegrationID: "redis",
	}
	first, err := StableProposalID(identity)
	if err != nil {
		t.Fatalf("StableProposalID() error = %v", err)
	}
	second, err := StableProposalID(identity)
	if err != nil {
		t.Fatalf("StableProposalID() repeated error = %v", err)
	}
	if first != second {
		t.Fatalf("StableProposalID() = %q then %q, want stable identity", first, second)
	}
	if first == "" {
		t.Fatal("StableProposalID() returned an empty ID")
	}
}
