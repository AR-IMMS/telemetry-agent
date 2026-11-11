package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrInvalidMatchSnapshot identifies malformed or unsafe matching input.
	ErrInvalidMatchSnapshot = errors.New("invalid integration match snapshot")
	// ErrInvalidCandidate identifies invalid or secret-bearing candidate settings.
	ErrInvalidCandidate = errors.New("invalid integration configuration candidate")
)

var matchFieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var matchProviderPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// MatchSelector describes exact, declarative resource evidence required by a definition.
type MatchSelector struct {
	ResourceKinds   []string          `json:"resource_kinds,omitempty"`
	LifecycleStates []string          `json:"lifecycle_states,omitempty"`
	ExactAttributes map[string]string `json:"exact_attributes,omitempty"`
}

// DefinitionMatcher performs a pure definition-owned match when selectors are insufficient.
type DefinitionMatcher func(MatchResource) (bool, []MatchEvidence, error)

// CandidateBuilder resolves observed evidence into non-secret configuration settings.
type CandidateBuilder func(MatchResource) (CandidateBuild, error)

// MatchingDefinition adds optional discovery matching to an existing exact-version definition.
type MatchingDefinition struct {
	Selector             *MatchSelector
	Match                DefinitionMatcher
	BuildCandidate       CandidateBuilder
	RequiredCapabilities []string
}

// CandidateBuild contains resolved settings and any non-secret inputs still required.
type CandidateBuild struct {
	Settings       map[string]any
	Evidence       []MatchEvidence
	RequiredInputs []RequiredInput
}

// MatchSnapshot is one consistent read-only inventory projection evaluated at a single time.
type MatchSnapshot struct {
	InventoryRevision uint64
	AsOf              time.Time
	StaleAfter        time.Duration
	Resources         []MatchResource
}

// MatchResource is the Integration-owned projection of provider-neutral inventory evidence.
type MatchResource struct {
	Provider, ProviderInstance, ResourceID string
	Kind, LifecycleState, Presence         string
	Attributes                             map[string]string
	Endpoints                              []MatchEndpoint
	LastSeenAt                             time.Time
}

// MatchEndpoint records endpoint scope and provenance without asserting reachability.
type MatchEndpoint struct {
	Address, Protocol, Scope, Provenance string
	Port                                 uint16
}

// ResourceIdentity is a provider-scoped resource key independent of Discovery types.
type ResourceIdentity struct {
	Provider, ProviderInstance, ResourceID string
}

// MatchEvidence explains a deterministic match or candidate resolution decision.
type MatchEvidence struct {
	Code, Source, Value string
}

// RequiredInput identifies a non-secret operator-supplied setting by name and reason.
type RequiredInput struct {
	Name, Reason string
}

// MatchStatus reports identification independently from configuration and capability.
type MatchStatus string

const (
	// MatchMatched means exactly one definition matched the resource.
	MatchMatched MatchStatus = "matched"
	// MatchNoMatch means no registered matching definition matched the resource.
	MatchNoMatch MatchStatus = "no_match"
	// MatchAmbiguous means more than one definition matched and none was selected.
	MatchAmbiguous MatchStatus = "ambiguous"
	// MatchStale means matching evidence is missing, stale or ineligible.
	MatchStale MatchStatus = "stale"
)

// ResolutionStatus reports whether matching evidence produced a valid configuration candidate.
type ResolutionStatus string

const (
	// ResolutionReady means settings passed the exact Integration Definition validator.
	ResolutionReady ResolutionStatus = "ready"
	// ResolutionNeedsInput means a required non-secret setting is not yet available.
	ResolutionNeedsInput ResolutionStatus = "needs_input"
	// ResolutionInvalid means candidate settings failed deterministic validation.
	ResolutionInvalid ResolutionStatus = "invalid"
)

// CapabilityStatus reports catalog compatibility separately from resource identification.
type CapabilityStatus string

const (
	// CapabilitySupported means every required capability was verified for the selected artifact.
	CapabilitySupported CapabilityStatus = "supported"
	// CapabilityUnsupported means the selected artifact lacks a required capability.
	CapabilityUnsupported CapabilityStatus = "unsupported"
	// CapabilityUnverified means capability evidence could not be established.
	CapabilityUnverified CapabilityStatus = "unverified"
	// CapabilityNotAssessed means the definition declares no runtime capability requirement.
	CapabilityNotAssessed CapabilityStatus = "not_assessed"
)

// CapabilityAssessment contains read-only evidence from the selected Collector catalog.
type CapabilityAssessment struct {
	Status                                     CapabilityStatus
	Platform, ArtifactVersion, ManifestVersion string
	Required, Unsupported                      []string
}

// CapabilityAssessor checks required IDs against the selected platform's verified catalog.
type CapabilityAssessor interface {
	Assess(required []string) (CapabilityAssessment, error)
}

// Candidate stores canonical, validated, secret-free configuration as immutable JSON bytes.
type Candidate struct {
	configuration []byte
	digest        string
}

// Configuration returns a defensive copy of the candidate configuration.
func (c Candidate) Configuration() json.RawMessage { return bytes.Clone(c.configuration) }

// Digest returns the stable candidate digest.
func (c Candidate) Digest() string { return c.digest }

// MatchDiagnostic is a safe, stable-coded per-report failure detail.
type MatchDiagnostic struct {
	Code, DefinitionID string
}

// MatchResult reports identification, resolution and capability as independent status axes.
type MatchResult struct {
	Resource            ResourceIdentity
	DefinitionID        string
	DefinitionVersion   string
	MatchStatus         MatchStatus
	ResolutionStatus    ResolutionStatus
	CapabilityStatus    CapabilityStatus
	ResourceFingerprint string
	Evidence            []MatchEvidence
	RequiredInputs      []RequiredInput
	Capability          CapabilityAssessment
	Candidate           *Candidate
	Diagnostics         []MatchDiagnostic
}

// MatchReport contains deterministically ordered results from one inventory revision.
type MatchReport struct {
	InventoryRevision uint64
	EvaluatedAt       time.Time
	Results           []MatchResult
	Diagnostics       []MatchDiagnostic
}

func validateMatchingDefinition(matching *MatchingDefinition) error {
	if matching == nil {
		return nil
	}
	if (matching.Selector == nil) == (matching.Match == nil) {
		return fmt.Errorf("matching definition requires exactly one selector or matcher")
	}
	if matching.Selector != nil {
		selector := matching.Selector
		if len(selector.ResourceKinds)+len(selector.LifecycleStates)+len(selector.ExactAttributes) == 0 || len(selector.ResourceKinds) > 32 || len(selector.LifecycleStates) > 32 || len(selector.ExactAttributes) > 64 {
			return fmt.Errorf("invalid selector limits")
		}
		if err := validateMatchFields(selector.ResourceKinds); err != nil {
			return err
		}
		if err := validateMatchFields(selector.LifecycleStates); err != nil {
			return err
		}
		for key, value := range selector.ExactAttributes {
			if key == "" || len(key) > 128 || len(value) > 1024 || containsControlText(key) || containsControlText(value) || sensitiveMatchKey(key) {
				return fmt.Errorf("invalid exact attribute selector")
			}
		}
	}
	if len(matching.RequiredCapabilities) > 64 {
		return fmt.Errorf("too many required capabilities")
	}
	seen := make(map[string]struct{}, len(matching.RequiredCapabilities))
	for _, capability := range matching.RequiredCapabilities {
		if !capabilityIDPattern.MatchString(capability) {
			return fmt.Errorf("invalid required capability")
		}
		if _, ok := seen[capability]; ok {
			return fmt.Errorf("duplicate required capability")
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func validateMatchFields(fields []string) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !matchFieldPattern.MatchString(field) {
			return fmt.Errorf("invalid matching field")
		}
		if _, ok := seen[field]; ok {
			return fmt.Errorf("duplicate matching field")
		}
		seen[field] = struct{}{}
	}
	return nil
}

func containsControlText(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}

func sensitiveMatchKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(key))
	for _, marker := range []string{"password", "secret", "token", "credential", "authorization", "privatekey", "apikey", "environment"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "env" || normalized == "args"
}
