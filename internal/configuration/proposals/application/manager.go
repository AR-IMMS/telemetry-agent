package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ProposalRepository stores proposal snapshots and compare-and-swap updates.
type ProposalRepository interface {
	// ProposalSnapshot returns a consistent, deterministic proposal view.
	ProposalSnapshot() ([]Proposal, error)
	// PutProposal inserts or updates one proposal with expected-revision concurrency.
	PutProposal(Proposal, uint64) error
	// DismissProposal records an explicit dismissal of the expected revision.
	DismissProposal(string, uint64, time.Time) error
}

// ProposalManager refreshes durable proposals from one complete deterministic match evaluation.
type ProposalManager struct{ repository ProposalRepository }

// NewProposalManager creates a proposal manager over the Configuration repository port.
func NewProposalManager(repository ProposalRepository) ProposalManager {
	return ProposalManager{repository: repository}
}

// Refresh persists only actionable or input-requiring matches and marks absent proposals stale.
func (m ProposalManager) Refresh(observations []ProposalObservation, at time.Time) error {
	if m.repository == nil || at.IsZero() {
		return fmt.Errorf("%w: repository and evaluation time are required", ErrInvalidProposal)
	}
	current, err := m.repository.ProposalSnapshot()
	if err != nil {
		return fmt.Errorf("read proposal snapshot: %w", err)
	}
	sortProposals(current)
	byID := make(map[string]Proposal, len(current))
	for _, proposal := range current {
		if _, exists := byID[proposal.ID]; exists {
			return fmt.Errorf("%w: duplicate persisted proposal ID", ErrInvalidProposal)
		}
		byID[proposal.ID] = cloneProposal(proposal)
	}
	seen := make(map[string]struct{}, len(observations))
	prepared := make(map[string]Proposal, len(observations))
	for _, observation := range observations {
		proposal, eligible, err := proposalFromObservation(observation, at)
		if err != nil {
			return err
		}
		if proposal.ID == "" {
			continue
		}
		seen[proposal.ID] = struct{}{}
		if previous, exists := prepared[proposal.ID]; exists {
			previous.Status = ProposalStale
			previous.Diagnostics = []string{"ambiguous_match"}
			prepared[proposal.ID] = previous
			continue
		}
		if !eligible {
			proposal.Status = ProposalStale
			proposal.Diagnostics = append(proposal.Diagnostics, "candidate_not_actionable")
		}
		prepared[proposal.ID] = proposal
	}

	preparedIDs := make([]string, 0, len(prepared))
	for id := range prepared {
		preparedIDs = append(preparedIDs, id)
	}
	sort.Strings(preparedIDs)
	for _, id := range preparedIDs {
		proposal := prepared[id]
		previous, exists := byID[id]
		if !exists {
			if proposal.Status == ProposalStale {
				continue
			}
			proposal.Revision = 1
			proposal.CreatedAt = at.UTC()
			proposal.UpdatedAt = at.UTC()
			if err := ValidateProposal(proposal); err != nil {
				return err
			}
			if err := m.repository.PutProposal(proposal, 0); err != nil {
				return fmt.Errorf("insert monitoring proposal: %w", err)
			}
			continue
		}
		previousStatus := previous.Status
		candidateChanged := !sameProposalMaterial(previous, proposal)
		if previousStatus == ProposalDismissed && !candidateChanged {
			continue
		}
		if previousStatus == ProposalApproved && !candidateChanged && proposal.Status != ProposalStale {
			continue
		}
		if previousStatus == ProposalApproved {
			proposal.ApprovedInstanceID = previous.ApprovedInstanceID
			proposal.ApprovedInstanceRevision = previous.ApprovedInstanceRevision
		}
		if !candidateChanged && previousStatus == proposal.Status {
			continue
		}
		if previous.Revision == ^uint64(0) {
			return fmt.Errorf("%w: proposal revision overflow", ErrInvalidProposal)
		}
		proposal.Revision = previous.Revision + 1
		proposal.CreatedAt = previous.CreatedAt
		proposal.UpdatedAt = at.UTC()
		if err := ValidateProposal(proposal); err != nil {
			return err
		}
		if err := m.repository.PutProposal(proposal, previous.Revision); err != nil {
			return fmt.Errorf("refresh monitoring proposal: %w", err)
		}
	}

	currentIDs := make([]string, 0, len(byID))
	for id := range byID {
		currentIDs = append(currentIDs, id)
	}
	sort.Strings(currentIDs)
	for _, id := range currentIDs {
		previous := byID[id]
		if _, exists := seen[id]; exists || previous.Status == ProposalStale {
			continue
		}
		if previous.Revision == ^uint64(0) {
			return fmt.Errorf("%w: proposal revision overflow", ErrInvalidProposal)
		}
		previous.Revision++
		previous.Status = ProposalStale
		previous.Diagnostics = []string{"resource_or_match_unavailable"}
		previous.UpdatedAt = at.UTC()
		if err := m.repository.PutProposal(previous, byID[id].Revision); err != nil {
			return fmt.Errorf("mark missing monitoring proposal stale: %w", err)
		}
	}
	return nil
}

// Dismiss records an explicit operator decision using the expected material revision.
func (m ProposalManager) Dismiss(id string, expectedRevision uint64, at time.Time) error {
	if m.repository == nil || id == "" || expectedRevision == 0 || at.IsZero() {
		return fmt.Errorf("%w: proposal ID, revision and time are required", ErrInvalidProposal)
	}
	proposals, err := m.repository.ProposalSnapshot()
	if err != nil {
		return fmt.Errorf("read proposal before dismissal: %w", err)
	}
	for _, proposal := range proposals {
		if proposal.ID != id {
			continue
		}
		if proposal.Revision != expectedRevision {
			return ErrProposalConflict
		}
		if proposal.Status == ProposalDismissed {
			return nil
		}
		if proposal.Status != ProposalPending && proposal.Status != ProposalNeedsInput {
			return ErrProposalTransition
		}
		if err := m.repository.DismissProposal(id, expectedRevision, at.UTC()); err != nil {
			return fmt.Errorf("dismiss monitoring proposal: %w", err)
		}
		return nil
	}
	return ErrUnknownProposal
}

func proposalFromObservation(observation ProposalObservation, at time.Time) (Proposal, bool, error) {
	if observation.MatchStatus == "no_match" || observation.Identity.IntegrationID == "" {
		return Proposal{}, false, nil
	}
	if !validProposalMatchStatus(observation.MatchStatus) || !validProposalResolutionStatus(observation.ResolutionStatus) || !validProposalCapabilityStatus(observation.CapabilityStatus) {
		return Proposal{}, false, fmt.Errorf("%w: invalid matcher status", ErrInvalidProposal)
	}
	id, err := StableProposalID(observation.Identity)
	if err != nil {
		return Proposal{}, false, err
	}
	if observation.DefinitionVersion == "" || len(observation.DefinitionVersion) > 64 || at.IsZero() {
		return Proposal{}, false, fmt.Errorf("%w: definition version and evaluation time are required", ErrInvalidProposal)
	}
	proposal := Proposal{
		ID: id, Identity: observation.Identity, Revision: 1, DefinitionVersion: observation.DefinitionVersion,
		ResourceFingerprint: observation.ResourceFingerprint, CandidateDigest: observation.CandidateDigest,
		CandidateConfiguration: bytes.Clone(observation.CandidateConfiguration),
		CapabilityEvidence:     bytes.Clone(observation.CapabilityEvidence),
		Evidence:               append([]ProposalEvidence(nil), observation.Evidence...),
		RequiredInputs:         append([]ProposalInput(nil), observation.RequiredInputs...),
		Diagnostics:            append([]string(nil), observation.Diagnostics...),
		InventoryRevision:      observation.InventoryRevision, Status: ProposalStale,
		CreatedAt: at.UTC(), UpdatedAt: at.UTC(),
	}
	sort.Slice(proposal.Evidence, func(i, j int) bool {
		left, right := proposal.Evidence[i], proposal.Evidence[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.Value < right.Value
	})
	sort.Slice(proposal.RequiredInputs, func(i, j int) bool {
		if proposal.RequiredInputs[i].Name == proposal.RequiredInputs[j].Name {
			return proposal.RequiredInputs[i].Reason < proposal.RequiredInputs[j].Reason
		}
		return proposal.RequiredInputs[i].Name < proposal.RequiredInputs[j].Name
	})
	sort.Strings(proposal.Diagnostics)
	if observation.MatchStatus == ProposalMatchMatched {
		switch {
		case observation.CapabilityStatus != ProposalCapabilitySupported && observation.CapabilityStatus != ProposalCapabilityNotAssessed:
			proposal.Diagnostics = append(proposal.Diagnostics, "capability_not_approvable")
		case observation.ResolutionStatus == ProposalResolutionNeedsInput:
			proposal.Status = ProposalNeedsInput
		case observation.ResolutionStatus == ProposalResolutionReady && len(proposal.CandidateConfiguration) > 0 && proposal.CandidateDigest != "":
			proposal.Status = ProposalPending
		default:
			proposal.Diagnostics = append(proposal.Diagnostics, "candidate_not_approvable")
		}
	}
	eligible := proposal.Status == ProposalPending || proposal.Status == ProposalNeedsInput
	if err := ValidateProposal(proposal); err != nil {
		return Proposal{}, false, err
	}
	return proposal, eligible, nil
}

func sameProposalMaterial(previous, next Proposal) bool {
	if previous.Identity != next.Identity || previous.DefinitionVersion != next.DefinitionVersion ||
		previous.ResourceFingerprint != next.ResourceFingerprint || previous.CandidateDigest != next.CandidateDigest ||
		!bytes.Equal(previous.CandidateConfiguration, next.CandidateConfiguration) || !bytes.Equal(previous.CapabilityEvidence, next.CapabilityEvidence) {
		return false
	}
	leftEvidence, _ := json.Marshal(previous.Evidence)
	rightEvidence, _ := json.Marshal(next.Evidence)
	leftInputs, _ := json.Marshal(previous.RequiredInputs)
	rightInputs, _ := json.Marshal(next.RequiredInputs)
	leftDiagnostics, _ := json.Marshal(previous.Diagnostics)
	rightDiagnostics, _ := json.Marshal(next.Diagnostics)
	return bytes.Equal(leftEvidence, rightEvidence) && bytes.Equal(leftInputs, rightInputs) && bytes.Equal(leftDiagnostics, rightDiagnostics)
}

func cloneProposal(proposal Proposal) Proposal {
	proposal.CandidateConfiguration = bytes.Clone(proposal.CandidateConfiguration)
	proposal.CapabilityEvidence = bytes.Clone(proposal.CapabilityEvidence)
	proposal.Evidence = append([]ProposalEvidence(nil), proposal.Evidence...)
	proposal.RequiredInputs = append([]ProposalInput(nil), proposal.RequiredInputs...)
	proposal.Diagnostics = append([]string(nil), proposal.Diagnostics...)
	return proposal
}

func sortProposals(proposals []Proposal) {
	sort.Slice(proposals, func(i, j int) bool { return proposals[i].ID < proposals[j].ID })
}
