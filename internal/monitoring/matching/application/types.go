package application

import (
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitions "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

// Registry is the exact-version registry consumed by the matcher.
type Registry = definitionapp.Registry

// IntegrationDefinition is the validated definition evaluated by Monitoring.
type IntegrationDefinition = definitions.Definition

// MatchSnapshot is one consistent read-only resource projection.
type MatchSnapshot = matchingdomain.MatchSnapshot

// MatchResource is one provider-scoped normalized resource.
type MatchResource = matchingdomain.MatchResource

// MatchEndpoint records endpoint scope and provenance.
type MatchEndpoint = matchingdomain.MatchEndpoint

// ResourceIdentity identifies one provider-scoped resource.
type ResourceIdentity = matchingdomain.ResourceIdentity

// MatchEvidence explains an observed matching decision.
type MatchEvidence = matchingdomain.MatchEvidence

// RequiredInput identifies one non-secret setting still needed.
type RequiredInput = matchingdomain.RequiredInput

// MatchSelector contains exact declarative matching rules.
type MatchSelector = matchingdomain.MatchSelector

// MatchStatus reports whether a definition matched the resource.
type MatchStatus = matchingdomain.MatchStatus

// ResolutionStatus reports whether matching produced a valid candidate.
type ResolutionStatus = matchingdomain.ResolutionStatus

// CapabilityStatus reports pinned Collector capability evidence.
type CapabilityStatus = matchingdomain.CapabilityStatus

// CapabilityAssessment contains Runtime catalog evidence for a definition.
type CapabilityAssessment = matchingdomain.CapabilityAssessment

// CapabilityAssessor queries required IDs from the pinned Runtime catalog.
type CapabilityAssessor = matchingdomain.CapabilityAssessor

// Candidate contains an immutable validated configuration payload.
type Candidate = matchingdomain.Candidate

// MatchDiagnostic is a stable-coded failure detail.
type MatchDiagnostic = matchingdomain.MatchDiagnostic

// MatchResult reports independent match, resolution and capability outcomes.
type MatchResult = matchingdomain.MatchResult

// MatchReport contains deterministically ordered results for one snapshot.
type MatchReport = matchingdomain.MatchReport

const (
	// MatchMatched means exactly one definition matched the resource.
	MatchMatched = matchingdomain.MatchMatched
	// MatchNoMatch means no definition matched the resource.
	MatchNoMatch = matchingdomain.MatchNoMatch
	// MatchAmbiguous means multiple definitions matched.
	MatchAmbiguous = matchingdomain.MatchAmbiguous
	// MatchStale means resource evidence was missing, stale or ineligible.
	MatchStale = matchingdomain.MatchStale
	// ResolutionReady means settings passed the exact definition validator.
	ResolutionReady = matchingdomain.ResolutionReady
	// ResolutionNeedsInput means a non-secret setting is missing.
	ResolutionNeedsInput = matchingdomain.ResolutionNeedsInput
	// ResolutionInvalid means candidate settings failed validation.
	ResolutionInvalid = matchingdomain.ResolutionInvalid
	// CapabilitySupported means every required capability was verified.
	CapabilitySupported = matchingdomain.CapabilitySupported
	// CapabilityUnsupported means the selected artifact lacks a required capability.
	CapabilityUnsupported = matchingdomain.CapabilityUnsupported
	// CapabilityUnverified means capability evidence was unavailable.
	CapabilityUnverified = matchingdomain.CapabilityUnverified
	// CapabilityNotAssessed means the definition requires no runtime capability.
	CapabilityNotAssessed = matchingdomain.CapabilityNotAssessed
)

var (
	// ErrInvalidMatchSnapshot identifies malformed or unsafe matching input.
	ErrInvalidMatchSnapshot = matchingdomain.ErrInvalidMatchSnapshot
	// ErrInvalidCandidate identifies invalid or secret-bearing candidate settings.
	ErrInvalidCandidate = matchingdomain.ErrInvalidCandidate
)

func candidatePointer(candidate Candidate) *Candidate { return &candidate }
func matchFieldName(value string) bool                { return matchingdomain.ValidFieldName(value) }
func matchProviderName(value string) bool             { return matchingdomain.ValidProviderName(value) }
func controlText(value string) bool                   { return matchingdomain.ContainsControlText(value) }
func sensitiveKey(value string) bool                  { return matchingdomain.SensitiveMatchKey(value) }

func candidateFromBytes(configuration []byte, digest string) *Candidate {
	candidate := matchingdomain.NewCandidate(configuration, digest)
	return &candidate
}
