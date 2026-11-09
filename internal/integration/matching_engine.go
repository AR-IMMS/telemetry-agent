package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// Matcher deterministically evaluates registered definitions against one inventory projection.
type Matcher struct {
	registry   Registry
	capability CapabilityAssessor
}

// NewMatcher creates a read-only matcher over the existing S1 registry.
func NewMatcher(registry Registry, capability CapabilityAssessor) Matcher {
	return Matcher{registry: registry, capability: capability}
}

// Match evaluates one consistent snapshot without persistence, provider calls or runtime effects.
func (m Matcher) Match(snapshot MatchSnapshot) (MatchReport, error) {
	resources, err := validateAndCopyMatchSnapshot(snapshot)
	if err != nil {
		return MatchReport{}, err
	}
	report := MatchReport{InventoryRevision: snapshot.InventoryRevision, EvaluatedAt: snapshot.AsOf, Results: make([]MatchResult, 0)}
	definitions := m.registry.MatchableDefinitions()
	for _, resource := range resources {
		matches := make([]MatchResult, 0)
		for _, definition := range definitions {
			matched, evidence, matchErr := evaluateDefinition(definition, resource)
			if matchErr != nil {
				report.Diagnostics = append(report.Diagnostics, MatchDiagnostic{Code: "match_evaluation_failed", DefinitionID: definition.ID})
				continue
			}
			if matched {
				matches = append(matches, m.resolveMatch(snapshot, resource, definition, evidence))
			}
		}
		if len(matches) > 1 {
			for i := range matches {
				matches[i].MatchStatus = MatchAmbiguous
				matches[i].ResolutionStatus = ResolutionNeedsInput
				matches[i].RequiredInputs = []RequiredInput{{Name: "integration", Reason: "more than one Integration Definition matches this resource"}}
				matches[i].Candidate = nil
			}
		}
		if len(matches) == 0 {
			report.Results = append(report.Results, MatchResult{
				Resource: identityOf(resource), MatchStatus: MatchNoMatch,
				ResolutionStatus: ResolutionNeedsInput, CapabilityStatus: CapabilityNotAssessed,
				ResourceFingerprint: resourceFingerprint(resource),
			})
			continue
		}
		report.Results = append(report.Results, matches...)
	}
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		if report.Diagnostics[i].DefinitionID == report.Diagnostics[j].DefinitionID {
			return report.Diagnostics[i].Code < report.Diagnostics[j].Code
		}
		return report.Diagnostics[i].DefinitionID < report.Diagnostics[j].DefinitionID
	})
	return report, nil
}

func (m Matcher) resolveMatch(snapshot MatchSnapshot, resource MatchResource, definition IntegrationDefinition, evidence []MatchEvidence) MatchResult {
	result := MatchResult{
		Resource: identityOf(resource), DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		MatchStatus: MatchMatched, ResolutionStatus: ResolutionNeedsInput, CapabilityStatus: CapabilityNotAssessed,
		ResourceFingerprint: resourceFingerprint(resource), Evidence: append([]MatchEvidence(nil), evidence...),
	}
	if resource.Presence != "present" || resource.LastSeenAt.IsZero() || resource.LastSeenAt.After(snapshot.AsOf) || snapshot.AsOf.Sub(resource.LastSeenAt) > snapshot.StaleAfter {
		result.MatchStatus = MatchStale
		result.RequiredInputs = []RequiredInput{{Name: "fresh_resource", Reason: "resource evidence is missing or stale"}}
		return result
	}
	if len(definition.Matching.RequiredCapabilities) > 0 {
		result.Capability = m.assessCapabilities(definition.Matching.RequiredCapabilities)
		result.CapabilityStatus = result.Capability.Status
	}
	if definition.Matching.BuildCandidate == nil {
		result.RequiredInputs = []RequiredInput{{Name: "configuration", Reason: "definition requires operator configuration"}}
		return result
	}
	built, err := definition.Matching.BuildCandidate(cloneMatchResource(resource))
	if err != nil {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_build_failed", DefinitionID: definition.ID}}
		return result
	}
	result.Evidence = append(result.Evidence, built.Evidence...)
	if !validEvidence(result.Evidence) {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_evidence_invalid", DefinitionID: definition.ID}}
		return result
	}
	sortEvidence(result.Evidence)
	inputs, validInputs := normalizeRequiredInputs(built.RequiredInputs)
	if !validInputs {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_input_invalid", DefinitionID: definition.ID}}
		return result
	}
	result.RequiredInputs = inputs
	if len(inputs) > 0 {
		return result
	}
	if built.Settings == nil {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_settings_missing", DefinitionID: definition.ID}}
		return result
	}
	encoded, err := json.Marshal(built.Settings)
	if err != nil || containsSecret(encoded) {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_settings_invalid", DefinitionID: definition.ID}}
		return result
	}
	var validatedSettings map[string]any
	if err := json.Unmarshal(encoded, &validatedSettings); err != nil {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_settings_invalid", DefinitionID: definition.ID}}
		return result
	}
	if err := definition.Validate(validatedSettings); err != nil {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_validation_failed", DefinitionID: definition.ID}}
		return result
	}
	encoded, err = json.Marshal(validatedSettings)
	if err != nil || containsSecret(encoded) {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_settings_invalid", DefinitionID: definition.ID}}
		return result
	}
	capabilities := append([]string(nil), definition.Matching.RequiredCapabilities...)
	sort.Strings(capabilities)
	candidatePayload, err := json.Marshal(struct {
		Resource     ResourceIdentity `json:"resource"`
		DefinitionID string           `json:"definition_id"`
		Version      string           `json:"definition_version"`
		Capabilities []string         `json:"capabilities"`
		Settings     json.RawMessage  `json:"settings"`
	}{identityOf(resource), definition.ID, definition.Version, capabilities, encoded})
	if err != nil {
		result.ResolutionStatus = ResolutionInvalid
		result.Diagnostics = []MatchDiagnostic{{Code: "candidate_digest_failed", DefinitionID: definition.ID}}
		return result
	}
	digest := sha256.Sum256(candidatePayload)
	result.Candidate = &Candidate{configuration: encoded, digest: "sha256:" + hex.EncodeToString(digest[:])}
	result.ResolutionStatus = ResolutionReady
	return result
}

func (m Matcher) assessCapabilities(required []string) CapabilityAssessment {
	if m.capability == nil {
		return CapabilityAssessment{Status: CapabilityUnverified, Required: append([]string(nil), required...)}
	}
	assessment, err := m.capability.Assess(append([]string(nil), required...))
	if err != nil || !validCapabilityStatus(assessment.Status) || assessment.Status == CapabilityNotAssessed {
		return CapabilityAssessment{Status: CapabilityUnverified, Required: append([]string(nil), required...)}
	}
	if assessment.Status == CapabilitySupported || assessment.Status == CapabilityUnsupported {
		if assessment.Platform == "" || assessment.ArtifactVersion == "" || assessment.ManifestVersion == "" {
			return CapabilityAssessment{Status: CapabilityUnverified, Required: append([]string(nil), required...)}
		}
		for _, unsupported := range assessment.Unsupported {
			if !contains(required, unsupported) {
				return CapabilityAssessment{Status: CapabilityUnverified, Required: append([]string(nil), required...)}
			}
		}
		if len(assessment.Unsupported) > 0 {
			assessment.Status = CapabilityUnsupported
		}
	}
	assessment.Required = append([]string(nil), required...)
	assessment.Unsupported = append([]string(nil), assessment.Unsupported...)
	sort.Strings(assessment.Unsupported)
	return assessment
}

func evaluateDefinition(definition IntegrationDefinition, resource MatchResource) (bool, []MatchEvidence, error) {
	matching := definition.Matching
	if matching.Selector != nil {
		return selectorMatches(*matching.Selector, resource)
	}
	matched, evidence, err := matching.Match(cloneMatchResource(resource))
	if err != nil {
		return false, nil, err
	}
	if !validEvidence(evidence) {
		return false, nil, fmt.Errorf("invalid match evidence")
	}
	sortEvidence(evidence)
	return matched, evidence, nil
}

func selectorMatches(selector MatchSelector, resource MatchResource) (bool, []MatchEvidence, error) {
	var evidence []MatchEvidence
	if len(selector.ResourceKinds) > 0 {
		if !contains(selector.ResourceKinds, resource.Kind) {
			return false, nil, nil
		}
		evidence = append(evidence, MatchEvidence{Code: "resource_kind_exact", Source: "kind", Value: resource.Kind})
	}
	if len(selector.LifecycleStates) > 0 {
		if !contains(selector.LifecycleStates, resource.LifecycleState) {
			return false, nil, nil
		}
		evidence = append(evidence, MatchEvidence{Code: "lifecycle_exact", Source: "lifecycle_state", Value: resource.LifecycleState})
	}
	keys := make([]string, 0, len(selector.ExactAttributes))
	for key := range selector.ExactAttributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		want := selector.ExactAttributes[key]
		value, ok := resource.Attributes[key]
		if !ok || value != want {
			return false, nil, nil
		}
		code := "attribute_exact"
		if strings.HasSuffix(key, ".repository") {
			code = "image_repository_exact"
		}
		evidence = append(evidence, MatchEvidence{Code: code, Source: key, Value: value})
	}
	sortEvidence(evidence)
	return true, evidence, nil
}

func validateAndCopyMatchSnapshot(snapshot MatchSnapshot) ([]MatchResource, error) {
	if snapshot.AsOf.IsZero() || snapshot.StaleAfter <= 0 || len(snapshot.Resources) > 10000 {
		return nil, fmt.Errorf("%w: invalid evaluation time or snapshot limits", ErrInvalidMatchSnapshot)
	}
	resources := make([]MatchResource, len(snapshot.Resources))
	for i, resource := range snapshot.Resources {
		if err := validateMatchResource(resource); err != nil {
			return nil, fmt.Errorf("%w: resource evidence is malformed", ErrInvalidMatchSnapshot)
		}
		resources[i] = cloneMatchResource(resource)
	}
	sort.Slice(resources, func(i, j int) bool {
		return identitySortKey(identityOf(resources[i])) < identitySortKey(identityOf(resources[j]))
	})
	for i := 1; i < len(resources); i++ {
		if identitySortKey(identityOf(resources[i-1])) == identitySortKey(identityOf(resources[i])) {
			return nil, fmt.Errorf("%w: duplicate provider-scoped resource", ErrInvalidMatchSnapshot)
		}
	}
	return resources, nil
}

func validateMatchResource(resource MatchResource) error {
	identity := identityOf(resource)
	if !matchProviderPattern.MatchString(identity.Provider) || identity.ProviderInstance == "" || len(identity.ProviderInstance) > 128 || strings.TrimSpace(identity.ProviderInstance) != identity.ProviderInstance || strings.ContainsAny(identity.ProviderInstance, "/\\") || identity.ResourceID == "" || len(identity.ResourceID) > 256 || containsControlText(identity.ProviderInstance) || containsControlText(identity.ResourceID) || strings.ContainsAny(identity.ResourceID, "/\\") {
		return fmt.Errorf("invalid provider-scoped identity")
	}
	if !matchFieldPattern.MatchString(resource.Kind) || len(resource.LifecycleState) > 64 || containsControlText(resource.LifecycleState) || (resource.Presence != "present" && resource.Presence != "missing") {
		return fmt.Errorf("invalid resource state")
	}
	if len(resource.Attributes) > 64 || len(resource.Endpoints) > 256 {
		return fmt.Errorf("matching evidence limit exceeded")
	}
	for key, value := range resource.Attributes {
		if key == "" || len(key) > 128 || len(value) > 1024 || containsControlText(key) || containsControlText(value) || sensitiveMatchKey(key) {
			return fmt.Errorf("unsafe resource attributes")
		}
	}
	for _, endpoint := range resource.Endpoints {
		if len(endpoint.Address) > 256 || len(endpoint.Protocol) > 16 || len(endpoint.Scope) > 32 || len(endpoint.Provenance) > 64 || containsControlText(endpoint.Address) || containsControlText(endpoint.Provenance) {
			return fmt.Errorf("invalid endpoint evidence")
		}
		if endpoint.Address != "" {
			if _, err := netip.ParseAddr(endpoint.Address); err != nil && strings.ContainsAny(endpoint.Address, " \t/@?#") {
				return fmt.Errorf("invalid endpoint address")
			}
		}
	}
	return nil
}

func cloneMatchResource(resource MatchResource) MatchResource {
	copyOfResource := resource
	copyOfResource.Attributes = make(map[string]string, len(resource.Attributes))
	for key, value := range resource.Attributes {
		copyOfResource.Attributes[key] = value
	}
	copyOfResource.Endpoints = append([]MatchEndpoint(nil), resource.Endpoints...)
	return copyOfResource
}

func resourceFingerprint(resource MatchResource) string {
	copyOfResource := cloneMatchResource(resource)
	sort.Slice(copyOfResource.Endpoints, func(i, j int) bool {
		a, b := copyOfResource.Endpoints[i], copyOfResource.Endpoints[j]
		return strings.Join([]string{a.Scope, a.Address, a.Protocol, a.Provenance, fmt.Sprint(a.Port)}, "\x00") < strings.Join([]string{b.Scope, b.Address, b.Protocol, b.Provenance, fmt.Sprint(b.Port)}, "\x00")
	})
	payload, _ := json.Marshal(struct {
		Identity                       ResourceIdentity `json:"identity"`
		Kind, LifecycleState, Presence string
		Attributes                     map[string]string
		Endpoints                      []MatchEndpoint
	}{identityOf(copyOfResource), copyOfResource.Kind, copyOfResource.LifecycleState, copyOfResource.Presence, copyOfResource.Attributes, copyOfResource.Endpoints})
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func identityOf(resource MatchResource) ResourceIdentity {
	return ResourceIdentity{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID}
}

func identitySortKey(identity ResourceIdentity) string {
	return identity.Provider + "\x00" + identity.ProviderInstance + "\x00" + identity.ResourceID
}

func normalizeRequiredInputs(inputs []RequiredInput) ([]RequiredInput, bool) {
	copyOfInputs := append([]RequiredInput(nil), inputs...)
	for _, input := range copyOfInputs {
		if !matchFieldPattern.MatchString(input.Name) || len(input.Reason) == 0 || len(input.Reason) > 256 || containsControlText(input.Reason) || sensitiveMatchKey(input.Name) {
			return nil, false
		}
	}
	sort.Slice(copyOfInputs, func(i, j int) bool {
		if copyOfInputs[i].Name == copyOfInputs[j].Name {
			return copyOfInputs[i].Reason < copyOfInputs[j].Reason
		}
		return copyOfInputs[i].Name < copyOfInputs[j].Name
	})
	return copyOfInputs, true
}

func validEvidence(evidence []MatchEvidence) bool {
	if len(evidence) > 128 {
		return false
	}
	for _, item := range evidence {
		if !matchFieldPattern.MatchString(item.Code) || item.Source == "" || len(item.Source) > 128 || len(item.Value) > 1024 || containsControlText(item.Source) || containsControlText(item.Value) || sensitiveMatchKey(item.Source) {
			return false
		}
	}
	return true
}

func sortEvidence(evidence []MatchEvidence) {
	sort.Slice(evidence, func(i, j int) bool {
		left := strings.Join([]string{evidence[i].Code, evidence[i].Source, evidence[i].Value}, "\x00")
		right := strings.Join([]string{evidence[j].Code, evidence[j].Source, evidence[j].Value}, "\x00")
		return left < right
	})
}

func validCapabilityStatus(status CapabilityStatus) bool {
	switch status {
	case CapabilitySupported, CapabilityUnsupported, CapabilityUnverified, CapabilityNotAssessed:
		return true
	default:
		return false
	}
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
