package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	templatesdomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	matchingapp "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/application"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	monitoringtemplatesapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	monitoringtemplatesdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/domain"
)

// InventorySnapshotReader exposes persisted observed evidence to Configuration without granting write ownership.
type InventorySnapshotReader interface {
	SnapshotContext(context.Context) (inventorydomain.InventorySnapshot, error)
}

// TemplateSnapshotReader exposes Configuration-owned Templates for monitoring recommendation evaluation.
type TemplateSnapshotReader interface {
	SnapshotContext(context.Context) (templatesapp.Snapshot, error)
}

// ProposalMatcher evaluates monitoring definitions against one normalized inventory snapshot.
type ProposalMatcher interface {
	Match(matchingapp.MatchSnapshot) (matchingapp.MatchReport, error)
}

// ProposalTemplateEvaluator determines which inventory/profile pairs already have enabled Template intent.
type ProposalTemplateEvaluator interface {
	MatchTemplates(matchingapp.MatchSnapshot, []monitoringtemplatesdomain.TemplateInput) (monitoringtemplatesapp.TemplateMatchReport, error)
}

// ProposalRefreshService coordinates Monitoring recommendations into Configuration's durable proposal lifecycle.
type ProposalRefreshService struct {
	inventory InventorySnapshotReader
	templates TemplateSnapshotReader
	manager   ProposalManager
	matcher   ProposalMatcher
	evaluator ProposalTemplateEvaluator
}

// NewProposalRefreshService composes the Configuration proposal workflow from Inventory and Monitoring ports.
func NewProposalRefreshService(
	inventory InventorySnapshotReader,
	templates TemplateSnapshotReader,
	manager ProposalManager,
	matcher ProposalMatcher,
	evaluator ProposalTemplateEvaluator,
) ProposalRefreshService {
	return ProposalRefreshService{inventory: inventory, templates: templates, manager: manager, matcher: matcher, evaluator: evaluator}
}

// Refresh evaluates current Inventory and Template evidence and persists proposals until ctx is canceled.
func (s ProposalRefreshService) Refresh(ctx context.Context) error {
	if ctx == nil || s.inventory == nil || s.templates == nil || s.matcher == nil || s.evaluator == nil {
		return fmt.Errorf("proposal refresh context, evidence readers, matcher, and evaluator are required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	inventory, err := s.inventory.SnapshotContext(ctx)
	if err != nil {
		return fmt.Errorf("read inventory for proposal refresh: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	templates, err := s.templates.SnapshotContext(ctx)
	if err != nil {
		return fmt.Errorf("read Templates for proposal refresh: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	matchSnapshot := matchingapp.ProjectInventory(inventory, time.Now().UTC())
	templateInputs, err := MonitoringTemplateInputs(templates.Templates)
	if err != nil {
		return fmt.Errorf("prepare Templates for proposal refresh: %w", err)
	}
	templateReport, err := s.evaluator.MatchTemplates(matchSnapshot, templateInputs)
	if err != nil {
		return fmt.Errorf("match Templates for proposal refresh: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	managed := materializableTemplatePairs(templateReport)
	report, err := s.matcher.Match(matchSnapshot)
	if err != nil {
		return fmt.Errorf("match inventory for proposal refresh: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	observations := make([]ProposalObservation, 0, len(report.Results))
	for _, result := range report.Results {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, autoManaged := managed[templatePairKey(result.Resource, result.DefinitionID)]; autoManaged {
			continue
		}
		observation, err := ProposalObservationFromMatch(result, report.InventoryRevision)
		if err != nil {
			return err
		}
		observations = append(observations, observation)
	}
	if err := s.manager.RefreshContext(ctx, observations, report.EvaluatedAt); err != nil {
		return fmt.Errorf("refresh durable proposals: %w", err)
	}
	return nil
}

func materializableTemplatePairs(report monitoringtemplatesapp.TemplateMatchReport) map[string]struct{} {
	pairs := make(map[string]struct{})
	for _, result := range report.Results {
		if result.Materializable || result.Conflict {
			pairs[templatePairKey(result.Resource, result.DefinitionID)] = struct{}{}
		}
	}
	return pairs
}

func templatePairKey(resource matchingdomain.ResourceIdentity, definitionID string) string {
	return resource.Provider + "\x00" + resource.ProviderInstance + "\x00" + resource.ResourceID + "\x00" + definitionID
}

// ProposalObservationFromMatch maps a Monitoring match into Configuration's durable proposal input.
func ProposalObservationFromMatch(result matchingdomain.MatchResult, inventoryRevision uint64) (ProposalObservation, error) {
	observation := ProposalObservation{
		Identity: ProposalIdentity{
			Provider: result.Resource.Provider, ProviderInstance: result.Resource.ProviderInstance,
			ResourceID: result.Resource.ResourceID, IntegrationID: result.DefinitionID,
		},
		DefinitionVersion: result.DefinitionVersion, MatchStatus: string(result.MatchStatus),
		ResolutionStatus: string(result.ResolutionStatus), CapabilityStatus: string(result.CapabilityStatus),
		ResourceFingerprint: result.ResourceFingerprint, Evidence: make([]ProposalEvidence, 0, len(result.Evidence)),
		RequiredInputs: make([]ProposalInput, 0, len(result.RequiredInputs)), InventoryRevision: inventoryRevision,
	}
	if result.Candidate != nil {
		observation.CandidateConfiguration = result.Candidate.Configuration()
		observation.CandidateDigest = result.Candidate.Digest()
	}
	capabilityEvidence, err := json.Marshal(result.Capability)
	if err != nil {
		return ProposalObservation{}, fmt.Errorf("encode capability evidence: %w", err)
	}
	observation.CapabilityEvidence = capabilityEvidence
	for _, evidence := range result.Evidence {
		observation.Evidence = append(observation.Evidence, ProposalEvidence{Code: evidence.Code, Source: evidence.Source, Value: evidence.Value})
	}
	for _, input := range result.RequiredInputs {
		observation.RequiredInputs = append(observation.RequiredInputs, ProposalInput{Name: input.Name, Reason: input.Reason})
	}
	for _, diagnostic := range result.Diagnostics {
		observation.Diagnostics = append(observation.Diagnostics, diagnostic.Code)
	}
	return observation, nil
}

// MonitoringTemplateInputs projects Configuration-owned Templates into Monitoring's matching input.
func MonitoringTemplateInputs(templates []templatesdomain.Template) ([]monitoringtemplatesdomain.TemplateInput, error) {
	inputs := make([]monitoringtemplatesdomain.TemplateInput, 0, len(templates))
	for _, template := range templates {
		if err := template.Validate(); err != nil {
			return nil, err
		}
		bindings := make([]monitoringtemplatesdomain.TemplateSettingBinding, len(template.Bindings))
		for index, binding := range template.Bindings {
			bindings[index] = monitoringtemplatesdomain.TemplateSettingBinding{
				Target: binding.Target,
				Source: monitoringtemplatesdomain.TemplateBindingSource{
					Kind:               monitoringtemplatesdomain.TemplateBindingSourceKind(binding.Source.Kind),
					Attribute:          binding.Source.Attribute,
					EndpointScope:      binding.Source.EndpointScope,
					EndpointProtocol:   binding.Source.EndpointProtocol,
					EndpointProvenance: binding.Source.EndpointProvenance,
				},
			}
		}
		inputs = append(inputs, monitoringtemplatesdomain.TemplateInput{
			ID: template.ID, Owner: string(template.Owner), Revision: template.Revision,
			Evaluate:     template.Status == templatesdomain.StatusEnabled,
			DefinitionID: template.DefinitionID, DefinitionVersion: template.DefinitionVersion,
			Selector: template.Selector, Settings: template.Settings, Bindings: bindings,
		})
	}
	return inputs, nil
}
