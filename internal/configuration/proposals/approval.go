package proposals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
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
	Verify(context.Context, Proposal, map[string]any) (VerifiedCandidate, error)
}

// ApprovalCommit is the validated state transition persisted atomically by the repository.
type ApprovalCommit struct {
	ProposalID              string
	ProposalRevision        uint64
	ExpectedDesiredRevision uint64
	ExpectedCandidateDigest string
	VerifiedCandidateDigest string
	InventoryWriteRevision  uint64
	Instance                domain.Instance
}

// ApprovalRepository commits proposal approval and desired Integration state in one transaction.
type ApprovalRepository interface {
	// CommitApproval atomically records the proposal outcome and desired instance.
	CommitApproval(ApprovalCommit) (domain.Instance, uint64, error)
}

// ApprovalService verifies a fresh candidate before requesting an atomic desired-state mutation.
type ApprovalService struct {
	proposals  ProposalRepository
	verifier   CandidateVerifier
	repository ApprovalRepository
}

// NewApprovalService wires proposal reads, current-evidence verification and atomic persistence.
func NewApprovalService(proposals ProposalRepository, verifier CandidateVerifier, repository ApprovalRepository) ApprovalService {
	return ApprovalService{proposals: proposals, verifier: verifier, repository: repository}
}

// Approve revalidates the selected proposal and commits its discovery-bound Integration instance.
func (s ApprovalService) Approve(ctx context.Context, request ApprovalRequest) (domain.Instance, uint64, error) {
	if ctx == nil || s.proposals == nil || s.repository == nil || request.ProposalID == "" || request.ProposalRevision == 0 {
		return domain.Instance{}, 0, fmt.Errorf("%w: repository, proposal ID and revision are required", ErrApprovalNotEligible)
	}
	proposalList, err := s.proposals.ProposalSnapshot()
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("read proposal before approval: %w", err)
	}
	var proposal Proposal
	for _, candidate := range proposalList {
		if candidate.ID == request.ProposalID {
			proposal = candidate
			break
		}
	}
	if proposal.ID == "" {
		return domain.Instance{}, 0, ErrUnknownProposal
	}
	if proposal.Revision != request.ProposalRevision {
		return domain.Instance{}, 0, ErrApprovalConflict
	}
	if proposal.Status == ProposalApproved {
		instance := domain.Instance{ID: proposal.ApprovedInstanceID}
		return s.repository.CommitApproval(ApprovalCommit{
			ProposalID: proposal.ID, ProposalRevision: proposal.Revision,
			ExpectedDesiredRevision: request.ExpectedDesiredRevision, ExpectedCandidateDigest: proposal.CandidateDigest,
			VerifiedCandidateDigest: proposal.ApprovedCandidateDigest, Instance: instance,
		})
	}
	if proposal.Status != ProposalPending && proposal.Status != ProposalNeedsInput || s.verifier == nil {
		return domain.Instance{}, 0, ErrApprovalNotEligible
	}
	inputs, err := cloneSettings(request.Inputs)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("copy approval inputs: %w", err)
	}
	verified, err := s.verifier.Verify(ctx, proposal, inputs)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("revalidate monitoring proposal: %w", err)
	}
	if verified.DefinitionVersion == "" || verified.CandidateDigest == "" || verified.Configuration == nil {
		return domain.Instance{}, 0, fmt.Errorf("%w: verifier returned incomplete candidate", ErrApprovalStale)
	}
	if verified.DefinitionVersion != proposal.DefinitionVersion {
		return domain.Instance{}, 0, ErrApprovalStale
	}
	if proposal.CandidateDigest != "" && verified.CandidateDigest != proposal.CandidateDigest {
		return domain.Instance{}, 0, ErrApprovalStale
	}
	settings, err := cloneSettings(verified.Configuration)
	if err != nil {
		return domain.Instance{}, 0, fmt.Errorf("copy verified Integration settings: %w", err)
	}
	instance := domain.Instance{
		ID: discoveredInstanceID(proposal.ID), DefinitionID: proposal.Identity.IntegrationID,
		DefinitionVersion: verified.DefinitionVersion, Configuration: settings,
		Source: domain.SourceDiscovery,
		Binding: &domain.ResourceBinding{
			Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID,
		},
		Enabled: true,
	}
	if err := instance.Validate(); err != nil {
		return domain.Instance{}, 0, fmt.Errorf("validate approved Integration: %w", err)
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
