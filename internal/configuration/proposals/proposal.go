// Package proposals preserves the historical Configuration proposals import path.
// Proposal models live in domain and workflow services live in application; this
// compatibility package owns no persistence, validation, or lifecycle rules.
package proposals

import application "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"

// ProposalStatus preserves the legacy import path for the proposal lifecycle status.
type ProposalStatus = application.ProposalStatus

const (
	// ProposalPending identifies a candidate ready for explicit review.
	ProposalPending = application.ProposalPending
	// ProposalNeedsInput identifies a candidate awaiting non-secret settings.
	ProposalNeedsInput = application.ProposalNeedsInput
	// ProposalDismissed identifies an explicitly declined material revision.
	ProposalDismissed = application.ProposalDismissed
	// ProposalApproved identifies an atomically approved desired-state transition.
	ProposalApproved = application.ProposalApproved
	// ProposalStale identifies a proposal that cannot be approved from current evidence.
	ProposalStale = application.ProposalStale
	// ProposalMatchMatched identifies a matching Monitoring definition.
	ProposalMatchMatched = application.ProposalMatchMatched
	// ProposalMatchAmbiguous identifies more than one matching definition.
	ProposalMatchAmbiguous = application.ProposalMatchAmbiguous
	// ProposalMatchStale identifies resource evidence that is not current.
	ProposalMatchStale = application.ProposalMatchStale
	// ProposalResolutionReady identifies a fully resolved candidate.
	ProposalResolutionReady = application.ProposalResolutionReady
	// ProposalResolutionNeedsInput identifies a candidate awaiting operator input.
	ProposalResolutionNeedsInput = application.ProposalResolutionNeedsInput
	// ProposalResolutionInvalid identifies a candidate rejected during resolution.
	ProposalResolutionInvalid = application.ProposalResolutionInvalid
	// ProposalCapabilitySupported identifies a Collector-supported profile.
	ProposalCapabilitySupported = application.ProposalCapabilitySupported
	// ProposalCapabilityUnsupported identifies a Collector-unsupported profile.
	ProposalCapabilityUnsupported = application.ProposalCapabilityUnsupported
	// ProposalCapabilityUnverified identifies an unverified capability result.
	ProposalCapabilityUnverified = application.ProposalCapabilityUnverified
	// ProposalCapabilityNotAssessed identifies an unevaluated capability.
	ProposalCapabilityNotAssessed = application.ProposalCapabilityNotAssessed
)

// ProposalIdentity is the stable Inventory resource/Integration pair.
type ProposalIdentity = application.ProposalIdentity

// ProposalEvidence is bounded, sanitized matching evidence safe for operator review.
type ProposalEvidence = application.ProposalEvidence

// ProposalInput describes a non-secret setting required before explicit approval.
type ProposalInput = application.ProposalInput

// Proposal stores the current material candidate revision and its last approval reference.
type Proposal = application.Proposal

// ProposalObservation is a provider-neutral view of one Monitoring Matcher result.
type ProposalObservation = application.ProposalObservation

// ProposalRepository stores proposal snapshots and compare-and-swap updates.
type ProposalRepository = application.ProposalRepository

// ProposalManager preserves the legacy path for proposal workflow use cases.
type ProposalManager = application.ProposalManager

// NewProposalManager creates a proposal application service over a repository port.
func NewProposalManager(repository ProposalRepository) ProposalManager {
	return application.NewProposalManager(repository)
}

// StableProposalID returns a deterministic ID independent of definition version or candidate revision.
func StableProposalID(identity ProposalIdentity) (string, error) {
	return application.StableProposalID(identity)
}

// ErrInvalidProposal identifies malformed or unsafe proposal input.
var ErrInvalidProposal = application.ErrInvalidProposal

// ErrProposalConflict identifies a proposal revision changed by another writer.
var ErrProposalConflict = application.ErrProposalConflict

// ErrUnknownProposal identifies a proposal ID that is not persisted.
var ErrUnknownProposal = application.ErrUnknownProposal

// ErrProposalTransition identifies an unsupported operator state transition.
var ErrProposalTransition = application.ErrProposalTransition
