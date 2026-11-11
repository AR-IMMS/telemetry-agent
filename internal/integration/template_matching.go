package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TemplateMatchResult is one resource/template evaluation before desired-state projection.
type TemplateMatchResult struct {
	TemplateID        string
	TemplateOwner     TemplateOwner
	TemplateRevision  uint64
	Resource          ResourceIdentity
	DefinitionID      string
	DefinitionVersion string
	MatchStatus       MatchStatus
	ResolutionStatus  ResolutionStatus
	CapabilityStatus  CapabilityStatus
	Capability        CapabilityAssessment
	Applicable        bool
	Conflict          bool
	Materializable    bool
	Candidate         *Candidate
	Instance          *IntegrationInstance
	Evidence          []MatchEvidence
	RequiredInputs    []RequiredInput
	Diagnostics       []MatchDiagnostic
}

// TemplateMatchReport contains the complete deterministic evaluation of enabled Templates.
type TemplateMatchReport struct {
	InventoryRevision uint64
	EvaluatedAt       time.Time
	Results           []TemplateMatchResult
	Diagnostics       []MatchDiagnostic
}

// MatchTemplates evaluates all enabled Templates before reporting any materializable instance.
// Conflicts are grouped across the full report, so input ordering cannot select a winner.
func (m Matcher) MatchTemplates(snapshot MatchSnapshot, templates []IntegrationTemplate) (TemplateMatchReport, error) {
	resources, err := validateAndCopyMatchSnapshot(snapshot)
	if err != nil {
		return TemplateMatchReport{}, err
	}
	if len(templates) > 10000 {
		return TemplateMatchReport{}, fmt.Errorf("%w: Template count exceeds limit", ErrInvalidTemplate)
	}
	ordered := make([]IntegrationTemplate, 0, len(templates))
	seen := make(map[string]struct{}, len(templates))
	for _, template := range templates {
		if err := ValidateIntegrationTemplate(template); err != nil {
			return TemplateMatchReport{}, err
		}
		key := string(template.Owner) + "\x00" + template.ID
		if _, exists := seen[key]; exists {
			return TemplateMatchReport{}, fmt.Errorf("%w: duplicate owner/template identity", ErrInvalidTemplate)
		}
		seen[key] = struct{}{}
		if template.Status == TemplateEnabled {
			copyOfTemplate, err := CloneIntegrationTemplate(template)
			if err != nil {
				return TemplateMatchReport{}, err
			}
			ordered = append(ordered, copyOfTemplate)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Owner != ordered[j].Owner {
			return ordered[i].Owner < ordered[j].Owner
		}
		return ordered[i].ID < ordered[j].ID
	})
	report := TemplateMatchReport{InventoryRevision: snapshot.InventoryRevision, EvaluatedAt: snapshot.AsOf, Results: []TemplateMatchResult{}}
	for _, template := range ordered {
		definition, err := m.registry.ResolveDefinition(template.DefinitionID, template.DefinitionVersion)
		if err != nil {
			report.Diagnostics = append(report.Diagnostics, MatchDiagnostic{Code: "template_definition_unavailable", DefinitionID: template.DefinitionID})
			continue
		}
		for _, resource := range resources {
			result, applicable := m.evaluateTemplate(snapshot, template, definition, resource)
			if applicable {
				report.Results = append(report.Results, result)
			}
		}
	}
	markTemplateConflicts(report.Results)
	sort.Slice(report.Results, func(i, j int) bool {
		left, right := report.Results[i], report.Results[j]
		leftKey := identitySortKey(left.Resource) + "\x00" + left.DefinitionID + "\x00" + string(left.TemplateOwner) + "\x00" + left.TemplateID
		rightKey := identitySortKey(right.Resource) + "\x00" + right.DefinitionID + "\x00" + string(right.TemplateOwner) + "\x00" + right.TemplateID
		return leftKey < rightKey
	})
	sort.Slice(report.Diagnostics, func(i, j int) bool {
		if report.Diagnostics[i].DefinitionID == report.Diagnostics[j].DefinitionID {
			return report.Diagnostics[i].Code < report.Diagnostics[j].Code
		}
		return report.Diagnostics[i].DefinitionID < report.Diagnostics[j].DefinitionID
	})
	return report, nil
}

func (m Matcher) evaluateTemplate(snapshot MatchSnapshot, template IntegrationTemplate, definition IntegrationDefinition, resource MatchResource) (TemplateMatchResult, bool) {
	result := TemplateMatchResult{
		TemplateID: template.ID, TemplateOwner: template.Owner, TemplateRevision: template.Revision,
		Resource: identityOf(resource), DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		MatchStatus: MatchMatched, ResolutionStatus: ResolutionNeedsInput, CapabilityStatus: CapabilityNotAssessed,
		Evidence: []MatchEvidence{}, Diagnostics: []MatchDiagnostic{},
	}
	if definition.Matching != nil {
		matched, evidence, err := evaluateDefinition(definition, resource)
		if err != nil {
			result.MatchStatus = MatchStale
			result.Diagnostics = append(result.Diagnostics, MatchDiagnostic{Code: "template_definition_match_failed", DefinitionID: definition.ID})
			return result, true
		}
		if !matched {
			return result, false
		}
		result.Evidence = append(result.Evidence, evidence...)
	} else if template.Selector == nil {
		result.MatchStatus = MatchStale
		result.Diagnostics = append(result.Diagnostics, MatchDiagnostic{Code: "template_selector_required", DefinitionID: definition.ID})
		return result, true
	}
	if template.Selector != nil {
		matched, evidence, err := selectorMatches(*template.Selector, resource)
		if err != nil {
			result.MatchStatus = MatchStale
			result.Diagnostics = append(result.Diagnostics, MatchDiagnostic{Code: "template_selector_invalid", DefinitionID: definition.ID})
			return result, true
		}
		if !matched {
			return result, false
		}
		result.Evidence = append(result.Evidence, evidence...)
	}
	result.Applicable = true
	if resource.Presence != "present" || resource.LastSeenAt.IsZero() || resource.LastSeenAt.After(snapshot.AsOf) || snapshot.AsOf.Sub(resource.LastSeenAt) > snapshot.StaleAfter {
		result.MatchStatus = MatchStale
		result.RequiredInputs = []RequiredInput{{Name: "fresh_resource", Reason: "resource evidence is missing or stale"}}
		return result, true
	}
	result.Resource = identityOf(resource)
	if definition.Matching != nil && len(definition.Matching.RequiredCapabilities) > 0 {
		result.Capability = m.assessCapabilities(definition.Matching.RequiredCapabilities)
		result.CapabilityStatus = result.Capability.Status
	}
	var built CandidateBuild
	if definition.Matching != nil && definition.Matching.BuildCandidate != nil {
		var err error
		built, err = definition.Matching.BuildCandidate(cloneMatchResource(resource))
		if err != nil {
			return templateResolutionFailure(result, "template_candidate_build_failed", ResolutionInvalid), true
		}
		if !validEvidence(built.Evidence) {
			return templateResolutionFailure(result, "template_candidate_evidence_invalid", ResolutionInvalid), true
		}
		result.Evidence = append(result.Evidence, built.Evidence...)
	}
	settings, err := ResolveTemplateSettings(template, definition, resource)
	if err != nil {
		if err == ErrTemplateValueUnavailable || strings.Contains(err.Error(), ErrTemplateValueUnavailable.Error()) {
			return templateResolutionFailure(result, "template_binding_unavailable", ResolutionNeedsInput), true
		}
		return templateResolutionFailure(result, "template_settings_invalid", ResolutionInvalid), true
	}
	if built.Settings != nil {
		encoded, err := json.Marshal(built.Settings)
		if err != nil || containsSecret(encoded) {
			return templateResolutionFailure(result, "template_candidate_settings_invalid", ResolutionInvalid), true
		}
		for key, value := range built.Settings {
			if existing, exists := settings[key]; exists {
				if !sameJSONValue(existing, value) {
					return templateResolutionFailure(result, "template_candidate_settings_conflict", ResolutionInvalid), true
				}
				continue
			}
			cloned, err := cloneJSONValue(value)
			if err != nil {
				return templateResolutionFailure(result, "template_candidate_settings_invalid", ResolutionInvalid), true
			}
			settings[key] = cloned
		}
	}
	inputs, valid := normalizeRequiredInputs(built.RequiredInputs)
	if !valid {
		return templateResolutionFailure(result, "template_candidate_input_invalid", ResolutionInvalid), true
	}
	for _, input := range inputs {
		if _, resolved := settings[input.Name]; !resolved {
			result.RequiredInputs = append(result.RequiredInputs, input)
		}
	}
	if len(result.RequiredInputs) > 0 {
		result.ResolutionStatus = ResolutionNeedsInput
		return result, true
	}
	encoded, err := json.Marshal(settings)
	if err != nil || containsSecret(encoded) {
		return templateResolutionFailure(result, "template_resolved_settings_invalid", ResolutionInvalid), true
	}
	if err := definition.Validate(settings); err != nil {
		return templateResolutionFailure(result, "template_definition_validation_failed", ResolutionInvalid), true
	}
	capabilities := []string{}
	if definition.Matching != nil {
		capabilities = append(capabilities, definition.Matching.RequiredCapabilities...)
	}
	sort.Strings(capabilities)
	payload, err := json.Marshal(struct {
		Resource     ResourceIdentity `json:"resource"`
		DefinitionID string           `json:"definition_id"`
		Version      string           `json:"definition_version"`
		Capabilities []string         `json:"capabilities"`
		Settings     json.RawMessage  `json:"settings"`
	}{identityOf(resource), definition.ID, definition.Version, capabilities, encoded})
	if err != nil {
		return templateResolutionFailure(result, "template_candidate_digest_failed", ResolutionInvalid), true
	}
	digest := sha256.Sum256(payload)
	result.Candidate = &Candidate{configuration: encoded, digest: "sha256:" + hex.EncodeToString(digest[:])}
	result.ResolutionStatus = ResolutionReady
	if result.CapabilityStatus != CapabilityUnsupported && result.CapabilityStatus != CapabilityUnverified {
		id, err := GeneratedInstanceID(template.Owner, template.ID, identityOf(resource), definition.ID)
		if err != nil {
			return templateResolutionFailure(result, "template_instance_identity_invalid", ResolutionInvalid), true
		}
		instance := IntegrationInstance{
			ID: id, DefinitionID: definition.ID, DefinitionVersion: definition.Version, Configuration: settings,
			Source: SourceTemplate, Binding: &ResourceBinding{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID},
			TemplateOwner: template.Owner, TemplateID: template.ID, TemplateRevision: template.Revision, Enabled: true,
		}
		if err := ValidateInstance(instance); err != nil {
			return templateResolutionFailure(result, "template_instance_invalid", ResolutionInvalid), true
		}
		result.Instance = &instance
		result.Materializable = true
	}
	return result, true
}

func templateResolutionFailure(result TemplateMatchResult, code string, status ResolutionStatus) TemplateMatchResult {
	result.ResolutionStatus = status
	result.Diagnostics = append(result.Diagnostics, MatchDiagnostic{Code: code, DefinitionID: result.DefinitionID})
	return result
}

func markTemplateConflicts(results []TemplateMatchResult) {
	groups := make(map[string][]int)
	for index, result := range results {
		if result.Applicable && result.MatchStatus == MatchMatched {
			key := identitySortKey(result.Resource) + "\x00" + result.DefinitionID
			groups[key] = append(groups[key], index)
		}
	}
	for _, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			results[index].Conflict = true
			results[index].Materializable = false
			results[index].Instance = nil
			results[index].MatchStatus = MatchAmbiguous
			results[index].Diagnostics = append(results[index].Diagnostics, MatchDiagnostic{Code: "template_conflict", DefinitionID: results[index].DefinitionID})
		}
	}
}
