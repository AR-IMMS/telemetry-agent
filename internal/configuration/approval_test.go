package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestApprovalServiceRevalidatesAndBuildsDiscoveryBoundInstance(t *testing.T) {
	proposal := approvalTestProposal(t, discovery.ProposalPending)
	proposals := &approvalProposalRepository{proposals: []discovery.Proposal{proposal}}
	verifier := &approvalCandidateVerifier{candidate: VerifiedCandidate{
		DefinitionVersion: proposal.DefinitionVersion,
		Configuration:     map[string]any{"endpoint": "127.0.0.1:6379"},
		CandidateDigest:   proposal.CandidateDigest, InventoryWriteRevision: 7,
	}}
	repository := &recordingApprovalRepository{}
	service := NewApprovalService(proposals, verifier, repository)
	inputs := map[string]any{"label": "reviewed"}
	instance, desiredRevision, err := service.Approve(context.Background(), ApprovalRequest{
		ProposalID: proposal.ID, ProposalRevision: proposal.Revision, ExpectedDesiredRevision: 4, Inputs: inputs,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputs["label"] = "mutated"
	if verifier.inputs["label"] != "reviewed" {
		t.Fatal("verifier input aliases request settings")
	}
	wantBinding := integration.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a"}
	if instance.Source != integration.SourceDiscovery || instance.Binding == nil || *instance.Binding != wantBinding || instance.Revision != 3 || desiredRevision != 5 {
		t.Fatalf("approved instance=%+v desired revision=%d", instance, desiredRevision)
	}
	if repository.commit.ExpectedDesiredRevision != 4 || repository.commit.InventoryWriteRevision != 7 || repository.commit.VerifiedCandidateDigest != proposal.CandidateDigest {
		t.Fatalf("approval commit=%+v", repository.commit)
	}
}

func TestApprovalServiceRejectsStaleCandidateWithoutPersistence(t *testing.T) {
	proposal := approvalTestProposal(t, discovery.ProposalPending)
	proposals := &approvalProposalRepository{proposals: []discovery.Proposal{proposal}}
	verifier := &approvalCandidateVerifier{candidate: VerifiedCandidate{
		DefinitionVersion: proposal.DefinitionVersion, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
		CandidateDigest: "sha256:" + approvalRepeatedHex('c'),
	}}
	repository := &recordingApprovalRepository{}
	service := NewApprovalService(proposals, verifier, repository)
	_, _, err := service.Approve(context.Background(), ApprovalRequest{ProposalID: proposal.ID, ProposalRevision: proposal.Revision})
	if !errors.Is(err, ErrApprovalStale) || repository.calls != 0 {
		t.Fatalf("stale candidate err=%v repository calls=%d", err, repository.calls)
	}
}

func approvalTestProposal(t *testing.T, status discovery.ProposalStatus) discovery.Proposal {
	t.Helper()
	identity := discovery.ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"}
	id, err := discovery.StableProposalID(identity)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	proposal := discovery.Proposal{
		ID: id, Identity: identity, Revision: 2, Status: status, DefinitionVersion: "1.0.0",
		ResourceFingerprint: "sha256:" + approvalRepeatedHex('a'), CandidateDigest: "sha256:" + approvalRepeatedHex('b'),
		CandidateConfiguration: json.RawMessage(`{"endpoint":"127.0.0.1:6379"}`),
		CapabilityEvidence:     json.RawMessage(`{"status":"supported"}`), CreatedAt: now, UpdatedAt: now,
	}
	if err := discovery.ValidateProposal(proposal); err != nil {
		t.Fatal(err)
	}
	return proposal
}

func approvalRepeatedHex(value byte) string {
	bytes := make([]byte, 64)
	for i := range bytes {
		bytes[i] = value
	}
	return string(bytes)
}

type approvalProposalRepository struct{ proposals []discovery.Proposal }

func (r *approvalProposalRepository) ProposalSnapshot() ([]discovery.Proposal, error) {
	return append([]discovery.Proposal(nil), r.proposals...), nil
}

func (*approvalProposalRepository) PutProposal(discovery.Proposal, uint64) error    { return nil }
func (*approvalProposalRepository) DismissProposal(string, uint64, time.Time) error { return nil }

type approvalCandidateVerifier struct {
	candidate VerifiedCandidate
	inputs    map[string]any
}

func (v *approvalCandidateVerifier) Verify(_ context.Context, _ discovery.Proposal, inputs map[string]any) (VerifiedCandidate, error) {
	v.inputs = inputs
	return v.candidate, nil
}

type recordingApprovalRepository struct {
	commit ApprovalCommit
	calls  int
}

func (r *recordingApprovalRepository) CommitApproval(commit ApprovalCommit) (integration.IntegrationInstance, uint64, error) {
	r.commit = commit
	r.calls++
	commit.Instance.Revision = 3
	return commit.Instance, 5, nil
}
