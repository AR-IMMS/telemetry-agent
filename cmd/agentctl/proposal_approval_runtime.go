package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	configurationtemplatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	configurationtemplatesdomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	matchingapp "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/application"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
	monitoringtemplatesapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	monitoringtemplatesdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	"runtime"
	"time"
)

type runtimeProposalRefresher struct {
	inventory         inventoryapp.InventoryRepository
	templates         configurationtemplatesapp.Repository
	manager           application.ProposalManager
	matcher           matchingapp.Matcher
	templateEvaluator monitoringtemplatesapp.Evaluator
}

func newRuntimeProposalRefresher(statePath string) (runtimeProposalRefresher, error) {
	store := newAgentSQLiteStore(statePath)
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return runtimeProposalRefresher{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor matchingdomain.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = matchingdomain.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	return runtimeProposalRefresher{
		inventory: store.InventoryRepository(), templates: store.TemplateRepository(), manager: application.NewProposalManager(store.ProposalRepository()),
		matcher: matchingapp.NewMatcher(registry, assessor), templateEvaluator: monitoringtemplatesapp.NewEvaluator(registry, assessor),
	}, nil
}

func (r runtimeProposalRefresher) Refresh(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := r.inventory.Snapshot()
	if err != nil {
		return fmt.Errorf("read inventory for proposal refresh: %w", err)
	}
	templates, err := r.templates.Snapshot()
	if err != nil {
		return fmt.Errorf("read Templates for proposal refresh: %w", err)
	}
	matchSnapshot := projectInventory(snapshot, time.Now().UTC())
	templateInputs, err := monitoringTemplateInputs(templates.Templates)
	if err != nil {
		return fmt.Errorf("match Templates for proposal refresh: %w", err)
	}
	templateReport, err := r.templateEvaluator.MatchTemplates(matchSnapshot, templateInputs)
	if err != nil {
		return fmt.Errorf("match Templates for proposal refresh: %w", err)
	}
	managed := materializableTemplatePairs(templateReport)
	report, err := r.matcher.Match(matchSnapshot)
	if err != nil {
		return fmt.Errorf("match inventory for proposal refresh: %w", err)
	}
	observations := make([]application.ProposalObservation, 0, len(report.Results))
	for _, result := range report.Results {
		if _, autoManaged := managed[templatePairKey(result.Resource, result.DefinitionID)]; autoManaged {
			continue
		}
		observation, err := proposalObservationFromMatch(result, report.InventoryRevision)
		if err != nil {
			return err
		}
		observations = append(observations, observation)
	}
	if err := r.manager.Refresh(observations, report.EvaluatedAt); err != nil {
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

func proposalObservationFromMatch(result matchingdomain.MatchResult, inventoryRevision uint64) (application.ProposalObservation, error) {
	observation := application.ProposalObservation{
		Identity: application.ProposalIdentity{
			Provider: result.Resource.Provider, ProviderInstance: result.Resource.ProviderInstance,
			ResourceID: result.Resource.ResourceID, IntegrationID: result.DefinitionID,
		},
		DefinitionVersion: result.DefinitionVersion, MatchStatus: string(result.MatchStatus),
		ResolutionStatus: string(result.ResolutionStatus), CapabilityStatus: string(result.CapabilityStatus),
		ResourceFingerprint: result.ResourceFingerprint, Evidence: make([]application.ProposalEvidence, 0, len(result.Evidence)),
		RequiredInputs: make([]application.ProposalInput, 0, len(result.RequiredInputs)), InventoryRevision: inventoryRevision,
	}
	if result.Candidate != nil {
		observation.CandidateConfiguration = result.Candidate.Configuration()
		observation.CandidateDigest = result.Candidate.Digest()
	}
	capabilityEvidence, err := json.Marshal(result.Capability)
	if err != nil {
		return application.ProposalObservation{}, fmt.Errorf("encode capability evidence: %w", err)
	}
	observation.CapabilityEvidence = capabilityEvidence
	for _, evidence := range result.Evidence {
		observation.Evidence = append(observation.Evidence, application.ProposalEvidence{Code: evidence.Code, Source: evidence.Source, Value: evidence.Value})
	}
	for _, input := range result.RequiredInputs {
		observation.RequiredInputs = append(observation.RequiredInputs, application.ProposalInput{Name: input.Name, Reason: input.Reason})
	}
	for _, diagnostic := range result.Diagnostics {
		observation.Diagnostics = append(observation.Diagnostics, diagnostic.Code)
	}
	return observation, nil
}

func monitoringTemplateInputs(templates []configurationtemplatesdomain.Template) ([]monitoringtemplatesdomain.TemplateInput, error) {
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
			Evaluate:     template.Status == configurationtemplatesdomain.StatusEnabled,
			DefinitionID: template.DefinitionID, DefinitionVersion: template.DefinitionVersion,
			Selector: template.Selector, Settings: template.Settings, Bindings: bindings,
		})
	}
	return inputs, nil
}

type runtimeApprovalVerifier struct {
	inventory inventoryapp.InventoryRepository
	matcher   matchingapp.Matcher
	resolver  definitionapp.Resolver
}

func newRuntimeApprovalService(statePath string) (application.ApprovalService, error) {
	store := newAgentSQLiteStore(statePath)
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return application.ApprovalService{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor matchingdomain.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = matchingdomain.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	matcher := matchingapp.NewMatcher(registry, assessor)
	verifier := runtimeApprovalVerifier{
		inventory: store.InventoryRepository(), matcher: matcher, resolver: definitionapp.NewResolver(registry),
	}
	return application.NewApprovalService(store.ProposalRepository(), verifier, store.ApprovalRepository()), nil
}

func (v runtimeApprovalVerifier) Verify(ctx context.Context, proposal application.Proposal, inputs map[string]any) (application.VerifiedCandidate, error) {
	if err := ctx.Err(); err != nil {
		return application.VerifiedCandidate{}, err
	}
	snapshot, err := v.inventory.Snapshot()
	if err != nil {
		return application.VerifiedCandidate{}, err
	}
	report, err := v.matcher.Match(projectInventory(snapshot, time.Now().UTC()))
	if err != nil {
		return application.VerifiedCandidate{}, err
	}
	var match *matchingdomain.MatchResult
	for i := range report.Results {
		result := &report.Results[i]
		resource := result.Resource
		if resource.Provider == proposal.Identity.Provider && resource.ProviderInstance == proposal.Identity.ProviderInstance &&
			resource.ResourceID == proposal.Identity.ResourceID && result.DefinitionID == proposal.Identity.IntegrationID {
			match = result
			break
		}
	}
	if match == nil || match.MatchStatus != matchingdomain.MatchMatched || match.ResourceFingerprint != proposal.ResourceFingerprint ||
		(match.CapabilityStatus != matchingdomain.CapabilitySupported && match.CapabilityStatus != matchingdomain.CapabilityNotAssessed) {
		return application.VerifiedCandidate{}, application.ErrApprovalStale
	}
	settings := map[string]any{}
	if match.Candidate != nil {
		if err := json.Unmarshal(match.Candidate.Configuration(), &settings); err != nil {
			return application.VerifiedCandidate{}, err
		}
	}
	allowedInputs := make(map[string]struct{}, len(proposal.RequiredInputs))
	for _, input := range proposal.RequiredInputs {
		allowedInputs[input.Name] = struct{}{}
	}
	for name, value := range inputs {
		if _, ok := allowedInputs[name]; !ok {
			return application.VerifiedCandidate{}, application.ErrApprovalStale
		}
		settings[name] = value
	}
	if match.ResolutionStatus == matchingdomain.ResolutionNeedsInput && len(inputs) == 0 {
		return application.VerifiedCandidate{}, application.ErrApprovalStale
	}
	validationInstance := desiredstatedomain.Instance{
		ID: "approval-validation", DefinitionID: match.DefinitionID, DefinitionVersion: match.DefinitionVersion,
		Configuration: settings, Enabled: true, Source: desiredstatedomain.SourceDiscovery,
		Binding: &desiredstatedomain.ResourceBinding{
			Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID,
		},
	}
	resolved, err := resolveApprovalSettings(v.resolver, validationInstance)
	if err != nil {
		return application.VerifiedCandidate{}, err
	}
	encoded, err := json.Marshal(resolved.Settings)
	if err != nil {
		return application.VerifiedCandidate{}, err
	}
	capabilities := append([]string(nil), match.Capability.Required...)
	if match.Candidate != nil && len(inputs) == 0 {
		if match.Candidate.Digest() != proposal.CandidateDigest {
			return application.VerifiedCandidate{}, application.ErrApprovalStale
		}
		return application.VerifiedCandidate{
			DefinitionVersion: resolved.Version, Configuration: resolved.Settings,
			CandidateDigest: match.Candidate.Digest(), InventoryWriteRevision: snapshot.WriteRevision,
		}, nil
	}
	candidatePayload, err := json.Marshal(struct {
		Resource     matchingdomain.ResourceIdentity `json:"resource"`
		DefinitionID string                          `json:"definition_id"`
		Version      string                          `json:"definition_version"`
		Capabilities []string                        `json:"capabilities"`
		Settings     json.RawMessage                 `json:"settings"`
	}{
		Resource:     matchingdomain.ResourceIdentity{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID},
		DefinitionID: match.DefinitionID, Version: match.DefinitionVersion, Capabilities: capabilities, Settings: encoded,
	})
	if err != nil {
		return application.VerifiedCandidate{}, err
	}
	digest := sha256.Sum256(candidatePayload)
	candidateDigest := "sha256:" + hex.EncodeToString(digest[:])
	if proposal.CandidateDigest != "" && candidateDigest != proposal.CandidateDigest {
		return application.VerifiedCandidate{}, application.ErrApprovalStale
	}
	return application.VerifiedCandidate{
		DefinitionVersion: resolved.Version, Configuration: resolved.Settings,
		CandidateDigest: candidateDigest, InventoryWriteRevision: snapshot.WriteRevision,
	}, nil
}

var _ application.CandidateVerifier = runtimeApprovalVerifier{}

func resolveApprovalSettings(resolver definitionapp.Resolver, instance desiredstatedomain.Instance) (definitionapp.Resolved, error) {
	if err := instance.Validate(); err != nil {
		return definitionapp.Resolved{}, err
	}
	resolved, err := resolver.Resolve(definitionapp.ResolveInput{
		DefinitionID: instance.DefinitionID, Version: instance.DefinitionVersion,
		Enabled: instance.Enabled, Settings: instance.Configuration,
	})
	if err == nil || errors.Is(err, definitionsdomain.ErrUnknownDefinition) || errors.Is(err, definitionsdomain.ErrInvalidContribution) {
		return resolved, err
	}
	return definitionapp.Resolved{}, fmt.Errorf("%w: %v", desiredstatedomain.ErrInvalidInstance, err)
}
