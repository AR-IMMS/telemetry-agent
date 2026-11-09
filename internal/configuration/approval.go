package configuration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

var (
	// ErrApprovalConflict indicates proposal or desired state changed during review.
	ErrApprovalConflict = errors.New("monitoring proposal approval conflict")
	// ErrApprovalNotEligible indicates the proposal cannot be approved in its current state.
	ErrApprovalNotEligible = errors.New("monitoring proposal is not eligible for approval")
	// ErrApprovalStale indicates verified resource or candidate evidence changed.
	ErrApprovalStale = errors.New("monitoring proposal evidence is stale")
	// ErrApprovalBindingConflict indicates another desired Integration already owns the resource.
	ErrApprovalBindingConflict = errors.New("resource is already bound to another desired Integration")
	// ErrApprovalDuplicate indicates an equivalent desired Integration already exists.
	ErrApprovalDuplicate = errors.New("equivalent desired Integration already exists")
)

// ApprovalRequest identifies the exact proposal and desired-state snapshot the operator reviewed.
type ApprovalRequest struct {
	ProposalID              string
	ProposalRevision        uint64
	ExpectedDesiredRevision uint64
	Inputs                  map[string]any
}

// VerifiedCandidate contains fresh, revalidated non-secret settings from the current inventory.
type VerifiedCandidate struct {
	DefinitionVersion      string
	Configuration          map[string]any
	CandidateDigest        string
	InventoryWriteRevision uint64
}

// CandidateVerifier re-matches a proposal against fresh inventory before approval.
type CandidateVerifier interface {
	// Verify re-evaluates the proposal using current inventory and non-secret inputs.
	Verify(context.Context, discovery.Proposal, map[string]any) (VerifiedCandidate, error)
}

// ApprovalCommit is the validated state transition persisted atomically by the repository.
type ApprovalCommit struct {
	ProposalID              string
	ProposalRevision        uint64
	ExpectedDesiredRevision uint64
	ExpectedCandidateDigest string
	VerifiedCandidateDigest string
	InventoryWriteRevision  uint64
	Instance                integration.IntegrationInstance
}

// ApprovalRepository commits proposal approval and desired Integration state in one transaction.
type ApprovalRepository interface {
	// CommitApproval atomically records the proposal outcome and desired instance.
	CommitApproval(ApprovalCommit) (integration.IntegrationInstance, uint64, error)
}

// ApprovalService verifies a fresh candidate before requesting an atomic desired-state mutation.
type ApprovalService struct {
	proposals  discovery.ProposalRepository
	verifier   CandidateVerifier
	repository ApprovalRepository
}

// NewApprovalService wires proposal reads, current-evidence verification and atomic persistence.
func NewApprovalService(proposals discovery.ProposalRepository, verifier CandidateVerifier, repository ApprovalRepository) ApprovalService {
	return ApprovalService{proposals: proposals, verifier: verifier, repository: repository}
}

// Approve revalidates the selected proposal and commits its discovery-bound Integration instance.
func (s ApprovalService) Approve(ctx context.Context, request ApprovalRequest) (integration.IntegrationInstance, uint64, error) {
	if ctx == nil || s.proposals == nil || s.repository == nil || request.ProposalID == "" || request.ProposalRevision == 0 {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("%w: repository, proposal ID and revision are required", ErrApprovalNotEligible)
	}
	proposals, err := s.proposals.ProposalSnapshot()
	if err != nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("read proposal before approval: %w", err)
	}
	var proposal discovery.Proposal
	for _, candidate := range proposals {
		if candidate.ID == request.ProposalID {
			proposal = candidate
			break
		}
	}
	if proposal.ID == "" {
		return integration.IntegrationInstance{}, 0, discovery.ErrUnknownProposal
	}
	if proposal.Revision != request.ProposalRevision {
		return integration.IntegrationInstance{}, 0, ErrApprovalConflict
	}
	if proposal.Status == discovery.ProposalApproved {
		instance := integration.IntegrationInstance{ID: proposal.ApprovedInstanceID}
		return s.repository.CommitApproval(ApprovalCommit{
			ProposalID: proposal.ID, ProposalRevision: proposal.Revision,
			ExpectedDesiredRevision: request.ExpectedDesiredRevision, ExpectedCandidateDigest: proposal.CandidateDigest,
			VerifiedCandidateDigest: proposal.ApprovedCandidateDigest, Instance: instance,
		})
	}
	if proposal.Status != discovery.ProposalPending && proposal.Status != discovery.ProposalNeedsInput || s.verifier == nil {
		return integration.IntegrationInstance{}, 0, ErrApprovalNotEligible
	}
	inputs, err := cloneSettings(request.Inputs)
	if err != nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("copy approval inputs: %w", err)
	}
	verified, err := s.verifier.Verify(ctx, proposal, inputs)
	if err != nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("revalidate monitoring proposal: %w", err)
	}
	if verified.DefinitionVersion == "" || verified.CandidateDigest == "" || verified.Configuration == nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("%w: verifier returned incomplete candidate", ErrApprovalStale)
	}
	if proposal.CandidateDigest != "" && verified.CandidateDigest != proposal.CandidateDigest {
		return integration.IntegrationInstance{}, 0, ErrApprovalStale
	}
	settings, err := cloneSettings(verified.Configuration)
	if err != nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("copy verified Integration settings: %w", err)
	}
	instance := integration.IntegrationInstance{
		ID: discoveredInstanceID(proposal.ID), DefinitionID: proposal.Identity.IntegrationID,
		DefinitionVersion: verified.DefinitionVersion, Configuration: settings,
		Source: integration.SourceDiscovery,
		Binding: &integration.ResourceBinding{
			Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID,
		},
		Enabled: true,
	}
	if err := integration.ValidateInstance(instance); err != nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("validate approved Integration: %w", err)
	}
	return s.repository.CommitApproval(ApprovalCommit{
		ProposalID: proposal.ID, ProposalRevision: proposal.Revision,
		ExpectedDesiredRevision: request.ExpectedDesiredRevision, ExpectedCandidateDigest: proposal.CandidateDigest,
		VerifiedCandidateDigest: verified.CandidateDigest, InventoryWriteRevision: verified.InventoryWriteRevision, Instance: instance,
	})
}

func discoveredInstanceID(proposalID string) string {
	digest := sha256.Sum256([]byte(proposalID))
	return "discovered-" + hex.EncodeToString(digest[:12])
}

func cloneSettings(settings map[string]any) (map[string]any, error) {
	if settings == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}
