package integration

import (
	"time"

	templatesapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	templatesdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/domain"
)

// TemplateMatchResult preserves the former Integration report shape while
// Monitoring owns evaluation and Configuration owns desired-state projection.
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

// TemplateMatchReport preserves the former Integration report shape for
// compatibility callers. New code should consume Monitoring's report directly.
type TemplateMatchReport struct {
	InventoryRevision uint64
	EvaluatedAt       time.Time
	Results           []TemplateMatchResult
	Diagnostics       []MatchDiagnostic
}

// MatchTemplates delegates evaluation to Monitoring and projects materializable
// candidates into the historical Configuration desired-state instance shape.
func (m Matcher) MatchTemplates(snapshot MatchSnapshot, templates []IntegrationTemplate) (TemplateMatchReport, error) {
	inputs := make([]templatesdomain.TemplateInput, 0, len(templates))
	for _, template := range templates {
		input, err := monitoringTemplateInput(template)
		if err != nil {
			return TemplateMatchReport{}, err
		}
		inputs = append(inputs, input)
	}
	monitoringReport, err := templatesapp.NewEvaluator(m.registry, m.capability).MatchTemplates(snapshot, inputs)
	if err != nil {
		return TemplateMatchReport{}, err
	}
	report := TemplateMatchReport{
		InventoryRevision: monitoringReport.InventoryRevision,
		EvaluatedAt:       monitoringReport.EvaluatedAt,
		Results:           make([]TemplateMatchResult, 0, len(monitoringReport.Results)),
		Diagnostics:       append([]MatchDiagnostic(nil), monitoringReport.Diagnostics...),
	}
	for _, evaluated := range monitoringReport.Results {
		result := TemplateMatchResult{
			TemplateID: evaluated.TemplateID, TemplateOwner: TemplateOwner(evaluated.TemplateOwner), TemplateRevision: evaluated.TemplateRevision,
			Resource: evaluated.Resource, DefinitionID: evaluated.DefinitionID, DefinitionVersion: evaluated.DefinitionVersion,
			MatchStatus: evaluated.MatchStatus, ResolutionStatus: evaluated.ResolutionStatus,
			CapabilityStatus: evaluated.CapabilityStatus, Capability: evaluated.Capability,
			Applicable: evaluated.Applicable, Conflict: evaluated.Conflict, Materializable: evaluated.Materializable,
			Candidate:      evaluated.Candidate,
			Evidence:       append([]MatchEvidence(nil), evaluated.Evidence...),
			RequiredInputs: append([]RequiredInput(nil), evaluated.RequiredInputs...),
			Diagnostics:    append([]MatchDiagnostic(nil), evaluated.Diagnostics...),
		}
		if evaluated.Materializable {
			result.Instance, err = materializedTemplateInstance(evaluated)
			if err != nil {
				result.Materializable = false
				result.ResolutionStatus = ResolutionInvalid
				result.Diagnostics = append(result.Diagnostics, MatchDiagnostic{Code: "template_instance_identity_invalid", DefinitionID: evaluated.DefinitionID})
			} else if validationErr := ValidateInstance(*result.Instance); validationErr != nil {
				result.Instance = nil
				result.Materializable = false
				result.ResolutionStatus = ResolutionInvalid
				result.Diagnostics = append(result.Diagnostics, MatchDiagnostic{Code: "template_instance_invalid", DefinitionID: evaluated.DefinitionID})
			}
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func materializedTemplateInstance(result templatesapp.TemplateMatchResult) (*IntegrationInstance, error) {
	if result.Candidate == nil || result.Settings == nil {
		return nil, ErrInvalidTemplate
	}
	owner := TemplateOwner(result.TemplateOwner)
	id, err := GeneratedInstanceID(owner, result.TemplateID, result.Resource, result.DefinitionID)
	if err != nil {
		return nil, err
	}
	instance := IntegrationInstance{
		ID: id, DefinitionID: result.DefinitionID, DefinitionVersion: result.DefinitionVersion,
		Configuration: result.Settings, Source: SourceTemplate,
		Binding:       &ResourceBinding{Provider: result.Resource.Provider, ProviderInstance: result.Resource.ProviderInstance, ResourceID: result.Resource.ResourceID},
		TemplateOwner: owner, TemplateID: result.TemplateID, TemplateRevision: result.TemplateRevision,
		Enabled: true,
	}
	return &instance, nil
}

func monitoringTemplateInput(template IntegrationTemplate) (templatesdomain.TemplateInput, error) {
	if err := template.Validate(); err != nil {
		return templatesdomain.TemplateInput{}, err
	}
	bindings := make([]templatesdomain.TemplateSettingBinding, len(template.Bindings))
	for index, binding := range template.Bindings {
		bindings[index] = templatesdomain.TemplateSettingBinding{
			Target: binding.Target,
			Source: templatesdomain.TemplateBindingSource{
				Kind:               templatesdomain.TemplateBindingSourceKind(binding.Source.Kind),
				Attribute:          binding.Source.Attribute,
				EndpointScope:      binding.Source.EndpointScope,
				EndpointProtocol:   binding.Source.EndpointProtocol,
				EndpointProvenance: binding.Source.EndpointProvenance,
			},
		}
	}
	return templatesdomain.TemplateInput{
		ID: template.ID, Owner: string(template.Owner), Revision: template.Revision,
		Evaluate: template.Status == TemplateEnabled, DefinitionID: template.DefinitionID,
		DefinitionVersion: template.DefinitionVersion, Selector: template.Selector,
		Settings: template.Settings, Bindings: bindings,
	}, nil
}
