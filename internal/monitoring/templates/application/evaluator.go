package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	matchingapp "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/application"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitions "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
	templatedomain "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/domain"
)

// TemplateMatchResult is one resource/template evaluation before Configuration
// projects eligible settings into durable desired state.
type TemplateMatchResult struct {
	TemplateID        string
	TemplateOwner     string
	TemplateRevision  uint64
	Resource          matchingdomain.ResourceIdentity
	DefinitionID      string
	DefinitionVersion string
	MatchStatus       matchingdomain.MatchStatus
	ResolutionStatus  matchingdomain.ResolutionStatus
	CapabilityStatus  matchingdomain.CapabilityStatus
	Capability        matchingdomain.CapabilityAssessment
	Applicable        bool
	Conflict          bool
	Materializable    bool
	Settings          map[string]any
	Candidate         *matchingdomain.Candidate
	Evidence          []matchingdomain.MatchEvidence
	RequiredInputs    []matchingdomain.RequiredInput
	Diagnostics       []matchingdomain.MatchDiagnostic
}

// TemplateMatchReport contains deterministic evaluation results for one Inventory revision.
type TemplateMatchReport struct {
	InventoryRevision uint64
	EvaluatedAt       time.Time
	Results           []TemplateMatchResult
	Diagnostics       []matchingdomain.MatchDiagnostic
}

// Evaluator matches Configuration-owned Templates against normalized Inventory
// evidence and Monitoring definitions. It never creates desired state or persists results.
type Evaluator struct {
	registry   definitionapp.Registry
	capability matchingdomain.CapabilityAssessor
}

// NewEvaluator creates a read-only Template evaluator over the exact-version profile registry.
func NewEvaluator(registry definitionapp.Registry, capability matchingdomain.CapabilityAssessor) Evaluator {
	return Evaluator{registry: registry, capability: capability}
}

// MatchTemplates evaluates enabled Templates against one consistent Inventory snapshot.
// Conflicts are grouped across the complete report, so input order cannot select a winner.
func (e Evaluator) MatchTemplates(snapshot matchingdomain.MatchSnapshot, templates []templatedomain.TemplateInput) (TemplateMatchReport, error) {
	resources, err := matchingapp.ValidateAndCopyMatchSnapshot(snapshot)
	if err != nil {
		return TemplateMatchReport{}, err
	}
	if len(templates) > 10000 {
		return TemplateMatchReport{}, fmt.Errorf("%w: Template count exceeds limit", definitions.ErrInvalidTemplate)
	}
	ordered := make([]templatedomain.TemplateInput, 0, len(templates))
	seen := make(map[string]struct{}, len(templates))
	for _, template := range templates {
		if err := validateTemplateInput(template); err != nil {
			return TemplateMatchReport{}, err
		}
		key := template.Owner + "\x00" + template.ID
		if _, exists := seen[key]; exists {
			return TemplateMatchReport{}, fmt.Errorf("%w: duplicate owner/template identity", definitions.ErrInvalidTemplate)
		}
		seen[key] = struct{}{}
		if template.Evaluate {
			copyOfTemplate, err := cloneTemplateInput(template)
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
		definition, err := e.registry.ResolveDefinition(template.DefinitionID, template.DefinitionVersion)
		if err != nil {
			report.Diagnostics = append(report.Diagnostics, matchingdomain.MatchDiagnostic{Code: "template_definition_unavailable", DefinitionID: template.DefinitionID})
			continue
		}
		for _, resource := range resources {
			result, applicable := e.evaluateTemplate(snapshot, template, definition, resource)
			if applicable {
				report.Results = append(report.Results, result)
			}
		}
	}
	markTemplateConflicts(report.Results)
	sort.Slice(report.Results, func(i, j int) bool {
		left, right := report.Results[i], report.Results[j]
		leftKey := matchingapp.IdentitySortKey(left.Resource) + "\x00" + left.DefinitionID + "\x00" + string(left.TemplateOwner) + "\x00" + left.TemplateID
		rightKey := matchingapp.IdentitySortKey(right.Resource) + "\x00" + right.DefinitionID + "\x00" + string(right.TemplateOwner) + "\x00" + right.TemplateID
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

func (e Evaluator) evaluateTemplate(snapshot matchingdomain.MatchSnapshot, template templatedomain.TemplateInput, definition definitions.Definition, resource matchingdomain.MatchResource) (TemplateMatchResult, bool) {
	result := TemplateMatchResult{
		TemplateID: template.ID, TemplateOwner: template.Owner, TemplateRevision: template.Revision,
		Resource: matchingapp.IdentityOf(resource), DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		MatchStatus: matchingdomain.MatchMatched, ResolutionStatus: matchingdomain.ResolutionNeedsInput,
		CapabilityStatus: matchingdomain.CapabilityNotAssessed,
		Evidence:         []matchingdomain.MatchEvidence{}, Diagnostics: []matchingdomain.MatchDiagnostic{},
	}
	if definition.Matching != nil {
		matched, evidence, err := matchingapp.EvaluateDefinition(definition, resource)
		if err != nil {
			result.MatchStatus = matchingdomain.MatchStale
			result.Diagnostics = append(result.Diagnostics, matchingdomain.MatchDiagnostic{Code: "template_definition_match_failed", DefinitionID: definition.ID})
			return result, true
		}
		if !matched {
			return result, false
		}
		result.Evidence = append(result.Evidence, evidence...)
	} else if template.Selector == nil {
		result.MatchStatus = matchingdomain.MatchStale
		result.Diagnostics = append(result.Diagnostics, matchingdomain.MatchDiagnostic{Code: "template_selector_required", DefinitionID: definition.ID})
		return result, true
	}
	if template.Selector != nil {
		matched, evidence, err := matchingapp.SelectorMatches(*template.Selector, resource)
		if err != nil {
			result.MatchStatus = matchingdomain.MatchStale
			result.Diagnostics = append(result.Diagnostics, matchingdomain.MatchDiagnostic{Code: "template_selector_invalid", DefinitionID: definition.ID})
			return result, true
		}
		if !matched {
			return result, false
		}
		result.Evidence = append(result.Evidence, evidence...)
	}
	result.Applicable = true
	if resource.Presence != "present" || resource.LastSeenAt.IsZero() || resource.LastSeenAt.After(snapshot.AsOf) || snapshot.AsOf.Sub(resource.LastSeenAt) > snapshot.StaleAfter {
		result.MatchStatus = matchingdomain.MatchStale
		result.RequiredInputs = []matchingdomain.RequiredInput{{Name: "fresh_resource", Reason: "resource evidence is missing or stale"}}
		return result, true
	}
	if definition.Matching != nil && len(definition.Matching.RequiredCapabilities) > 0 {
		result.Capability = matchingapp.NewMatcher(e.registry, e.capability).AssessCapabilities(definition.Matching.RequiredCapabilities)
		result.CapabilityStatus = result.Capability.Status
	}
	var built matchingdomain.CandidateBuild
	if definition.Matching != nil && definition.Matching.BuildCandidate != nil {
		var err error
		built, err = definition.Matching.BuildCandidate(matchingapp.CloneMatchResource(resource))
		if err != nil {
			return templateResolutionFailure(result, "template_candidate_build_failed", matchingdomain.ResolutionInvalid), true
		}
		if !matchingapp.ValidEvidence(built.Evidence) {
			return templateResolutionFailure(result, "template_candidate_evidence_invalid", matchingdomain.ResolutionInvalid), true
		}
		result.Evidence = append(result.Evidence, built.Evidence...)
	}
	settings, err := ResolveSettings(template, definition, resource)
	if err != nil {
		if errors.Is(err, ErrTemplateValueUnavailable) {
			return templateResolutionFailure(result, "template_binding_unavailable", matchingdomain.ResolutionNeedsInput), true
		}
		return templateResolutionFailure(result, "template_settings_invalid", matchingdomain.ResolutionInvalid), true
	}
	if built.Settings != nil {
		encoded, err := json.Marshal(built.Settings)
		if err != nil || definitions.ContainsSecret(encoded) {
			return templateResolutionFailure(result, "template_candidate_settings_invalid", matchingdomain.ResolutionInvalid), true
		}
		for key, value := range built.Settings {
			if existing, exists := settings[key]; exists {
				if !sameJSONValue(existing, value) {
					return templateResolutionFailure(result, "template_candidate_settings_conflict", matchingdomain.ResolutionInvalid), true
				}
				continue
			}
			cloned, err := cloneJSONValue(value)
			if err != nil {
				return templateResolutionFailure(result, "template_candidate_settings_invalid", matchingdomain.ResolutionInvalid), true
			}
			settings[key] = cloned
		}
	}
	inputs, valid := matchingapp.NormalizeRequiredInputs(built.RequiredInputs)
	if !valid {
		return templateResolutionFailure(result, "template_candidate_input_invalid", matchingdomain.ResolutionInvalid), true
	}
	for _, input := range inputs {
		if _, resolved := settings[input.Name]; !resolved {
			result.RequiredInputs = append(result.RequiredInputs, input)
		}
	}
	if len(result.RequiredInputs) > 0 {
		result.ResolutionStatus = matchingdomain.ResolutionNeedsInput
		return result, true
	}
	encoded, err := json.Marshal(settings)
	if err != nil || definitions.ContainsSecret(encoded) {
		return templateResolutionFailure(result, "template_resolved_settings_invalid", matchingdomain.ResolutionInvalid), true
	}
	if err := definition.Validate(settings); err != nil {
		return templateResolutionFailure(result, "template_definition_validation_failed", matchingdomain.ResolutionInvalid), true
	}
	capabilities := []string{}
	if definition.Matching != nil {
		capabilities = append(capabilities, definition.Matching.RequiredCapabilities...)
	}
	sort.Strings(capabilities)
	payload, err := json.Marshal(struct {
		Resource     matchingdomain.ResourceIdentity `json:"resource"`
		DefinitionID string                          `json:"definition_id"`
		Version      string                          `json:"definition_version"`
		Capabilities []string                        `json:"capabilities"`
		Settings     json.RawMessage                 `json:"settings"`
	}{matchingapp.IdentityOf(resource), definition.ID, definition.Version, capabilities, encoded})
	if err != nil {
		return templateResolutionFailure(result, "template_candidate_digest_failed", matchingdomain.ResolutionInvalid), true
	}
	digest := sha256.Sum256(payload)
	candidate := matchingdomain.NewCandidate(encoded, "sha256:"+hex.EncodeToString(digest[:]))
	result.Candidate = &candidate
	result.Settings = settings
	result.ResolutionStatus = matchingdomain.ResolutionReady
	if result.CapabilityStatus != matchingdomain.CapabilityUnsupported && result.CapabilityStatus != matchingdomain.CapabilityUnverified {
		result.Materializable = true
	}
	return result, true
}

func cloneTemplateInput(template templatedomain.TemplateInput) (templatedomain.TemplateInput, error) {
	encoded, err := json.Marshal(template)
	if err != nil {
		return templatedomain.TemplateInput{}, fmt.Errorf("copy Monitoring Template evaluation input: %w", err)
	}
	var cloned templatedomain.TemplateInput
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return templatedomain.TemplateInput{}, fmt.Errorf("decode copied Monitoring Template evaluation input: %w", err)
	}
	return cloned, nil
}

func templateResolutionFailure(result TemplateMatchResult, code string, status matchingdomain.ResolutionStatus) TemplateMatchResult {
	result.ResolutionStatus = status
	result.Diagnostics = append(result.Diagnostics, matchingdomain.MatchDiagnostic{Code: code, DefinitionID: result.DefinitionID})
	return result
}

func markTemplateConflicts(results []TemplateMatchResult) {
	groups := make(map[string][]int)
	for index, result := range results {
		if result.Applicable && result.MatchStatus == matchingdomain.MatchMatched {
			key := matchingapp.IdentitySortKey(result.Resource) + "\x00" + result.DefinitionID
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
			results[index].MatchStatus = matchingdomain.MatchAmbiguous
			results[index].Diagnostics = append(results[index].Diagnostics, matchingdomain.MatchDiagnostic{Code: "template_conflict", DefinitionID: results[index].DefinitionID})
		}
	}
}
