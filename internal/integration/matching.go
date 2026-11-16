package integration

import (
	"regexp"

	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
)

var (
	ErrInvalidMatchSnapshot = matchingdomain.ErrInvalidMatchSnapshot
	ErrInvalidCandidate     = matchingdomain.ErrInvalidCandidate
)

var matchFieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var matchProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type MatchSelector = matchingdomain.MatchSelector
type DefinitionMatcher = matchingdomain.DefinitionMatcher
type CandidateBuilder = matchingdomain.CandidateBuilder
type MatchingDefinition = matchingdomain.MatchingDefinition
type CandidateBuild = matchingdomain.CandidateBuild
type MatchSnapshot = matchingdomain.MatchSnapshot
type MatchResource = matchingdomain.MatchResource
type MatchEndpoint = matchingdomain.MatchEndpoint
type ResourceIdentity = matchingdomain.ResourceIdentity
type MatchEvidence = matchingdomain.MatchEvidence
type RequiredInput = matchingdomain.RequiredInput
type MatchStatus = matchingdomain.MatchStatus
type ResolutionStatus = matchingdomain.ResolutionStatus
type CapabilityStatus = matchingdomain.CapabilityStatus
type CapabilityAssessment = matchingdomain.CapabilityAssessment
type CapabilityAssessor = matchingdomain.CapabilityAssessor
type CapabilityCatalog = matchingdomain.CapabilityCatalog
type Candidate = matchingdomain.Candidate
type MatchDiagnostic = matchingdomain.MatchDiagnostic
type MatchResult = matchingdomain.MatchResult
type MatchReport = matchingdomain.MatchReport

const (
	MatchMatched          = matchingdomain.MatchMatched
	MatchNoMatch          = matchingdomain.MatchNoMatch
	MatchAmbiguous        = matchingdomain.MatchAmbiguous
	MatchStale            = matchingdomain.MatchStale
	ResolutionReady       = matchingdomain.ResolutionReady
	ResolutionNeedsInput  = matchingdomain.ResolutionNeedsInput
	ResolutionInvalid     = matchingdomain.ResolutionInvalid
	CapabilitySupported   = matchingdomain.CapabilitySupported
	CapabilityUnsupported = matchingdomain.CapabilityUnsupported
	CapabilityUnverified  = matchingdomain.CapabilityUnverified
	CapabilityNotAssessed = matchingdomain.CapabilityNotAssessed
)

// NewCatalogCapabilityAssessor adapts Runtime's narrow capability query to legacy callers.
func NewCatalogCapabilityAssessor(catalog CapabilityCatalog, platformOS, platform string) CapabilityAssessor {
	return matchingdomain.NewCatalogCapabilityAssessor(catalog, platformOS, platform)
}

func candidateFromBytes(configuration []byte, digest string) *Candidate {
	candidate := matchingdomain.NewCandidate(configuration, digest)
	return &candidate
}
