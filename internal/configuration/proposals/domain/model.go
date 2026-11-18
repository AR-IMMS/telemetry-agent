package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
)

var (
	// ErrInvalidProposal identifies malformed or unsafe proposal input.
	ErrInvalidProposal = errors.New("invalid monitoring proposal")
	// ErrProposalConflict identifies a proposal revision changed by another writer.
	ErrProposalConflict = errors.New("monitoring proposal revision conflict")
	// ErrUnknownProposal identifies a proposal ID that is not persisted.
	ErrUnknownProposal = errors.New("monitoring proposal not found")
	// ErrProposalTransition identifies an unsupported operator state transition.
	ErrProposalTransition = errors.New("invalid monitoring proposal transition")
)

var integrationProposalIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ProposalStatus represents the review lifecycle owned by Configuration.
type ProposalStatus string

const (
	// ProposalPending means a fresh candidate is ready for explicit operator review.
	ProposalPending ProposalStatus = "pending"
	// ProposalNeedsInput means a match requires non-secret operator configuration.
	ProposalNeedsInput ProposalStatus = "needs_input"
	// ProposalDismissed means the operator declined the current material revision.
	ProposalDismissed ProposalStatus = "dismissed"
	// ProposalApproved means approval and desired-state mutation committed atomically.
	ProposalApproved ProposalStatus = "approved"
	// ProposalStale means current evidence cannot be approved.
	ProposalStale ProposalStatus = "stale"
)

const (
	// ProposalMatchMatched identifies a matching Monitoring definition.
	ProposalMatchMatched = "matched"
	// ProposalMatchAmbiguous identifies more than one matching definition.
	ProposalMatchAmbiguous = "ambiguous"
	// ProposalMatchStale identifies resource evidence that is not current.
	ProposalMatchStale = "stale"
	// ProposalResolutionReady identifies a fully resolved Monitoring candidate.
	ProposalResolutionReady = "ready"
	// ProposalResolutionNeedsInput identifies an incomplete operator-configurable candidate.
	ProposalResolutionNeedsInput = "needs_input"
	// ProposalResolutionInvalid identifies a candidate rejected during resolution.
	ProposalResolutionInvalid = "invalid"
	// ProposalCapabilitySupported identifies a profile supported by the pinned Collector.
	ProposalCapabilitySupported = "supported"
	// ProposalCapabilityUnsupported identifies a profile unsupported by the pinned Collector.
	ProposalCapabilityUnsupported = "unsupported"
	// ProposalCapabilityUnverified identifies capability evidence that could not be verified.
	ProposalCapabilityUnverified = "unverified"
	// ProposalCapabilityNotAssessed identifies a profile without a capability assessment.
	ProposalCapabilityNotAssessed = "not_assessed"
	maxProposalPayloadBytes       = 64 * 1024
)

// ProposalIdentity is the stable Inventory resource/Integration pair;
// definition version is revision data.
type ProposalIdentity struct {
	Provider, ProviderInstance, ResourceID, IntegrationID string
}

// StableProposalID returns a deterministic ID independent of definition version or candidate revision.
func StableProposalID(identity ProposalIdentity) (string, error) {
	key := inventorydomain.ResourceKey{Provider: inventorydomain.ProviderID(identity.Provider), ProviderInstance: inventorydomain.ProviderInstanceID(identity.ProviderInstance), ProviderResourceID: identity.ResourceID}
	if err := key.Validate(); err != nil || !integrationProposalIDPattern.MatchString(identity.IntegrationID) {
		return "", fmt.Errorf("%w: malformed proposal identity", ErrInvalidProposal)
	}
	canonical, err := json.Marshal(struct {
		Provider, ProviderInstance, ResourceID, IntegrationID string
	}{identity.Provider, identity.ProviderInstance, identity.ResourceID, identity.IntegrationID})
	if err != nil {
		return "", fmt.Errorf("%w: encode proposal identity: %v", ErrInvalidProposal, err)
	}
	digest := sha256.Sum256(canonical)
	return "proposal-" + hex.EncodeToString(digest[:]), nil
}

// ProposalEvidence is bounded, sanitized matching evidence safe for operator review.
type ProposalEvidence struct {
	Code, Source, Value string
}

// ProposalInput describes a non-secret setting required before explicit approval.
type ProposalInput struct {
	Name, Reason string
}

// Proposal stores the current material candidate revision and its last approval reference.
type Proposal struct {
	ID                       string
	Identity                 ProposalIdentity
	Revision                 uint64
	Status                   ProposalStatus
	DefinitionVersion        string
	ResourceFingerprint      string
	CandidateDigest          string
	ApprovedCandidateDigest  string
	CandidateConfiguration   json.RawMessage `json:",omitempty"`
	CapabilityEvidence       json.RawMessage
	Evidence                 []ProposalEvidence
	RequiredInputs           []ProposalInput
	Diagnostics              []string
	InventoryRevision        uint64
	ApprovedInstanceID       string
	ApprovedInstanceRevision uint64
	CreatedAt, UpdatedAt     time.Time
}
