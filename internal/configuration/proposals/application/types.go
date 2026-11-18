package application

import (
	"encoding/json"

	proposaldomain "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/domain"
)

// Proposal is the Configuration domain entity used by proposal use cases.
type Proposal = proposaldomain.Proposal

// ProposalStatus is the Configuration-owned proposal lifecycle status.
type ProposalStatus = proposaldomain.ProposalStatus

// ProposalIdentity is the stable Inventory resource/Integration pair.
type ProposalIdentity = proposaldomain.ProposalIdentity

// ProposalEvidence is bounded, sanitized matching evidence safe for operator review.
type ProposalEvidence = proposaldomain.ProposalEvidence

// ProposalInput describes a non-secret setting required before explicit approval.
type ProposalInput = proposaldomain.ProposalInput

// ProposalObservation is a provider-neutral view of one Monitoring Matcher result.
type ProposalObservation struct {
	Identity               ProposalIdentity
	DefinitionVersion      string
	MatchStatus            string
	ResolutionStatus       string
	CapabilityStatus       string
	ResourceFingerprint    string
	CandidateDigest        string
	CandidateConfiguration json.RawMessage
	CapabilityEvidence     json.RawMessage
	Evidence               []ProposalEvidence
	RequiredInputs         []ProposalInput
	Diagnostics            []string
	InventoryRevision      uint64
}

var (
	// ErrInvalidProposal identifies malformed or unsafe proposal input.
	ErrInvalidProposal = proposaldomain.ErrInvalidProposal
	// ErrProposalConflict identifies a proposal revision changed by another writer.
	ErrProposalConflict = proposaldomain.ErrProposalConflict
	// ErrUnknownProposal identifies a proposal ID that is not persisted.
	ErrUnknownProposal = proposaldomain.ErrUnknownProposal
	// ErrProposalTransition identifies an unsupported operator state transition.
	ErrProposalTransition = proposaldomain.ErrProposalTransition
)

const (
	// ProposalPending identifies a candidate ready for explicit review.
	ProposalPending = proposaldomain.ProposalPending
	// ProposalNeedsInput identifies a candidate awaiting non-secret settings.
	ProposalNeedsInput = proposaldomain.ProposalNeedsInput
	// ProposalDismissed identifies an explicitly declined material revision.
	ProposalDismissed = proposaldomain.ProposalDismissed
	// ProposalApproved identifies an atomically approved desired-state transition.
	ProposalApproved = proposaldomain.ProposalApproved
	// ProposalStale identifies a proposal that cannot be approved from current evidence.
	ProposalStale = proposaldomain.ProposalStale
	// ProposalMatchMatched identifies a matching Monitoring definition.
	ProposalMatchMatched = proposaldomain.ProposalMatchMatched
	// ProposalMatchAmbiguous identifies more than one matching definition.
	ProposalMatchAmbiguous = proposaldomain.ProposalMatchAmbiguous
	// ProposalMatchStale identifies resource evidence that is not current.
	ProposalMatchStale = proposaldomain.ProposalMatchStale
	// ProposalResolutionReady identifies a fully resolved candidate.
	ProposalResolutionReady = proposaldomain.ProposalResolutionReady
	// ProposalResolutionNeedsInput identifies a candidate awaiting operator input.
	ProposalResolutionNeedsInput = proposaldomain.ProposalResolutionNeedsInput
	// ProposalResolutionInvalid identifies a candidate rejected during resolution.
	ProposalResolutionInvalid = proposaldomain.ProposalResolutionInvalid
	// ProposalCapabilitySupported identifies a Collector-supported profile.
	ProposalCapabilitySupported = proposaldomain.ProposalCapabilitySupported
	// ProposalCapabilityUnsupported identifies a Collector-unsupported profile.
	ProposalCapabilityUnsupported = proposaldomain.ProposalCapabilityUnsupported
	// ProposalCapabilityUnverified identifies an unverified capability result.
	ProposalCapabilityUnverified = proposaldomain.ProposalCapabilityUnverified
	// ProposalCapabilityNotAssessed identifies an unevaluated capability.
	ProposalCapabilityNotAssessed = proposaldomain.ProposalCapabilityNotAssessed
)

// StableProposalID delegates to the domain's deterministic identity rule.
func StableProposalID(identity ProposalIdentity) (string, error) {
	return proposaldomain.StableProposalID(identity)
}

// ValidateProposal delegates persistence-safety invariants to the domain.
func ValidateProposal(proposal Proposal) error {
	return proposaldomain.ValidateProposal(proposal)
}

func validProposalMatchStatus(status string) bool {
	return proposaldomain.ValidProposalMatchStatus(status)
}

func validProposalResolutionStatus(status string) bool {
	return proposaldomain.ValidProposalResolutionStatus(status)
}

func validProposalCapabilityStatus(status string) bool {
	return proposaldomain.ValidProposalCapabilityStatus(status)
}
