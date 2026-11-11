package discovery

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestProposalManagerRefreshIsStableAndVersionsMaterialChanges(t *testing.T) {
	repository := newMemoryProposalRepository()
	manager := NewProposalManager(repository)
	now := time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)
	observation := testProposalObservation()

	if err := manager.Refresh([]ProposalObservation{observation}, now); err != nil {
		t.Fatal(err)
	}
	first := onlyProposal(t, repository)
	if first.Revision != 1 || first.Status != ProposalPending {
		t.Fatalf("initial proposal = %+v", first)
	}
	if err := manager.Refresh([]ProposalObservation{observation}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	unchanged := onlyProposal(t, repository)
	if unchanged.Revision != first.Revision || !unchanged.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("unchanged refresh advanced proposal: before=%+v after=%+v", first, unchanged)
	}

	changed := observation
	changed.DefinitionVersion = "1.1.0"
	changed.ResourceFingerprint = "sha256:" + repeatedHex('b')
	changed.CandidateDigest = "sha256:" + repeatedHex('c')
	changed.CandidateConfiguration = json.RawMessage(`{"endpoint":"127.0.0.1:6381"}`)
	if err := manager.Refresh([]ProposalObservation{changed}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	updated := onlyProposal(t, repository)
	if updated.ID != first.ID || updated.Revision != 2 || updated.DefinitionVersion != "1.1.0" || updated.Status != ProposalPending {
		t.Fatalf("material update = %+v", updated)
	}
}

func TestProposalManagerDismissesIdempotentlyUntilMaterialChange(t *testing.T) {
	repository := newMemoryProposalRepository()
	manager := NewProposalManager(repository)
	now := time.Now().UTC()
	if err := manager.Refresh([]ProposalObservation{testProposalObservation()}, now); err != nil {
		t.Fatal(err)
	}
	proposal := onlyProposal(t, repository)
	if err := manager.Dismiss(proposal.ID, proposal.Revision, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Dismiss(proposal.ID, proposal.Revision, now.Add(2*time.Second)); err != nil {
		t.Fatalf("repeat dismiss should be idempotent: %v", err)
	}
	dismissed := onlyProposal(t, repository)
	if dismissed.Status != ProposalDismissed || dismissed.Revision != proposal.Revision {
		t.Fatalf("dismissed proposal = %+v", dismissed)
	}
	if err := manager.Refresh([]ProposalObservation{testProposalObservation()}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := onlyProposal(t, repository); got.Status != ProposalDismissed || got.Revision != proposal.Revision {
		t.Fatalf("identical refresh resurrected dismissed proposal: %+v", got)
	}
	changed := testProposalObservation()
	changed.CandidateDigest = "sha256:" + repeatedHex('d')
	changed.CandidateConfiguration = json.RawMessage(`{"endpoint":"127.0.0.1:6382"}`)
	if err := manager.Refresh([]ProposalObservation{changed}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := onlyProposal(t, repository); got.Status != ProposalPending || got.Revision != proposal.Revision+1 {
		t.Fatalf("material change did not create a reviewable proposal revision: %+v", got)
	}
}

func TestProposalManagerDoesNotCreateUnsupportedOrAmbiguousProposals(t *testing.T) {
	repository := newMemoryProposalRepository()
	manager := NewProposalManager(repository)
	unsupported := testProposalObservation()
	unsupported.CapabilityStatus = ProposalCapabilityUnsupported
	ambiguous := testProposalObservation()
	ambiguous.MatchStatus = ProposalMatchAmbiguous
	if err := manager.Refresh([]ProposalObservation{unsupported, ambiguous}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if got, err := repository.ProposalSnapshot(); err != nil || len(got) != 0 {
		t.Fatalf("non-actionable observations persisted: proposals=%+v err=%v", got, err)
	}
}

func TestProposalManagerMarksUnseenProposalStaleWithoutDeletingApprovalEvidence(t *testing.T) {
	repository := newMemoryProposalRepository()
	manager := NewProposalManager(repository)
	now := time.Now().UTC()
	observation := testProposalObservation()
	if err := manager.Refresh([]ProposalObservation{observation}, now); err != nil {
		t.Fatal(err)
	}
	proposal := onlyProposal(t, repository)
	proposal.Status = ProposalApproved
	proposal.ApprovedInstanceID = "auto-test"
	proposal.ApprovedInstanceRevision = 3
	repository.proposals[proposal.ID] = proposal
	if err := manager.Refresh(nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got := onlyProposal(t, repository)
	if got.Status != ProposalStale || got.ApprovedInstanceID != "auto-test" || got.ApprovedInstanceRevision != 3 {
		t.Fatalf("stale transition lost approval evidence: %+v", got)
	}
}

func TestProposalStableIdentityIsProviderScopedAndVersionIndependent(t *testing.T) {
	first, err := StableProposalID(ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := StableProposalID(ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"})
	if err != nil || first != second {
		t.Fatalf("same logical identity differs: %q %q err=%v", first, second, err)
	}
	otherProvider, err := StableProposalID(ProposalIdentity{Provider: "process", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"})
	if err != nil || first == otherProvider {
		t.Fatalf("provider-scoped identities collided: %q %q err=%v", first, otherProvider, err)
	}
}

func TestProposalValidationRejectsSecretBearingPayloadAndMalformedVersion(t *testing.T) {
	proposal := testProposalObservation()
	prepared, eligible, err := proposalFromObservation(proposal, time.Now().UTC())
	if err != nil || !eligible {
		t.Fatalf("fixture proposal=%+v eligible=%t err=%v", prepared, eligible, err)
	}
	prepared.CandidateConfiguration = json.RawMessage(`{"endpoint":"127.0.0.1:6379","password":"plaintext"}`)
	if err := ValidateProposal(prepared); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("secret candidate validation error=%v", err)
	}
	prepared.CandidateConfiguration = json.RawMessage(`{"endpoint":"127.0.0.1:6379"}`)
	prepared.DefinitionVersion = "latest"
	if err := ValidateProposal(prepared); !errors.Is(err, ErrInvalidProposal) {
		t.Fatalf("malformed definition version error=%v", err)
	}
}

func testProposalObservation() ProposalObservation {
	return ProposalObservation{
		Identity:          ProposalIdentity{Provider: "docker", ProviderInstance: "local-engine", ResourceID: "container-a", IntegrationID: "redis"},
		DefinitionVersion: "1.0.0", MatchStatus: ProposalMatchMatched, ResolutionStatus: ProposalResolutionReady,
		CapabilityStatus:    ProposalCapabilitySupported,
		ResourceFingerprint: "sha256:" + repeatedHex('a'), CandidateDigest: "sha256:" + repeatedHex('b'),
		CandidateConfiguration: json.RawMessage(`{"endpoint":"127.0.0.1:6380"}`),
		CapabilityEvidence:     json.RawMessage(`{"status":"supported","artifact":"0.160.0"}`),
		InventoryRevision:      9, Evidence: []ProposalEvidence{{Code: "image_repository_exact", Source: "container.image.repository", Value: "docker.io/library/redis"}},
	}
}

func repeatedHex(value byte) string {
	bytes := make([]byte, 64)
	for i := range bytes {
		bytes[i] = value
	}
	return string(bytes)
}

func onlyProposal(t *testing.T, repository *memoryProposalRepository) Proposal {
	t.Helper()
	proposals, err := repository.ProposalSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 1 {
		t.Fatalf("proposal count = %d, want 1", len(proposals))
	}
	return proposals[0]
}

type memoryProposalRepository struct {
	mu        sync.Mutex
	proposals map[string]Proposal
}

func newMemoryProposalRepository() *memoryProposalRepository {
	return &memoryProposalRepository{proposals: make(map[string]Proposal)}
}

func (r *memoryProposalRepository) ProposalSnapshot() ([]Proposal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Proposal, 0, len(r.proposals))
	for _, proposal := range r.proposals {
		result = append(result, cloneProposal(proposal))
	}
	return result, nil
}

func (r *memoryProposalRepository) PutProposal(proposal Proposal, expectedRevision uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, exists := r.proposals[proposal.ID]
	if (!exists && expectedRevision != 0) || (exists && current.Revision != expectedRevision) {
		return ErrProposalConflict
	}
	r.proposals[proposal.ID] = cloneProposal(proposal)
	return nil
}

func (r *memoryProposalRepository) DismissProposal(id string, expectedRevision uint64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	proposal, exists := r.proposals[id]
	if !exists {
		return ErrUnknownProposal
	}
	if proposal.Revision != expectedRevision {
		return ErrProposalConflict
	}
	proposal.Status = ProposalDismissed
	proposal.UpdatedAt = at
	r.proposals[id] = proposal
	return nil
}
