package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	"runtime"
	"time"
)

type runtimeProposalRefresher struct {
	inventory inventoryctx.InventoryRepository
	templates configuration.TemplateRepository
	manager   application.ProposalManager
	matcher   integration.Matcher
}

func newRuntimeProposalRefresher(statePath string) (runtimeProposalRefresher, error) {
	store := newAgentSQLiteStore(statePath)
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return runtimeProposalRefresher{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor integration.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = integration.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	return runtimeProposalRefresher{
		inventory: store.InventoryRepository(), templates: store.TemplateRepository(), manager: application.NewProposalManager(store.ProposalRepository()),
		matcher: integration.NewMatcher(registry, assessor),
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
	templateReport, err := r.matcher.MatchTemplates(matchSnapshot, templates.Templates)
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

func materializableTemplatePairs(report integration.TemplateMatchReport) map[string]struct{} {
	pairs := make(map[string]struct{})
	for _, result := range report.Results {
		if result.Materializable || result.Conflict {
			pairs[templatePairKey(result.Resource, result.DefinitionID)] = struct{}{}
		}
	}
	return pairs
}

func templatePairKey(resource integration.ResourceIdentity, definitionID string) string {
	return resource.Provider + "\x00" + resource.ProviderInstance + "\x00" + resource.ResourceID + "\x00" + definitionID
}

func proposalObservationFromMatch(result integration.MatchResult, inventoryRevision uint64) (application.ProposalObservation, error) {
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

type runtimeApprovalVerifier struct {
	inventory inventoryctx.InventoryRepository
	matcher   integration.Matcher
	resolver  integration.Resolver
}

func newRuntimeApprovalService(statePath string) (application.ApprovalService, error) {
	store := newAgentSQLiteStore(statePath)
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return application.ApprovalService{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor integration.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = integration.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	matcher := integration.NewMatcher(registry, assessor)
	verifier := runtimeApprovalVerifier{
		inventory: store.InventoryRepository(), matcher: matcher, resolver: integration.NewResolver(registry),
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
	var match *integration.MatchResult
	for i := range report.Results {
		result := &report.Results[i]
		resource := result.Resource
		if resource.Provider == proposal.Identity.Provider && resource.ProviderInstance == proposal.Identity.ProviderInstance &&
			resource.ResourceID == proposal.Identity.ResourceID && result.DefinitionID == proposal.Identity.IntegrationID {
			match = result
			break
		}
	}
	if match == nil || match.MatchStatus != integration.MatchMatched || match.ResourceFingerprint != proposal.ResourceFingerprint ||
		(match.CapabilityStatus != integration.CapabilitySupported && match.CapabilityStatus != integration.CapabilityNotAssessed) {
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
	if match.ResolutionStatus == integration.ResolutionNeedsInput && len(inputs) == 0 {
		return application.VerifiedCandidate{}, application.ErrApprovalStale
	}
	resolved, err := v.resolver.Resolve(integration.IntegrationInstance{
		ID: "approval-validation", DefinitionID: match.DefinitionID, DefinitionVersion: match.DefinitionVersion,
		Configuration: settings, Enabled: true, Source: integration.SourceDiscovery,
		Binding: &integration.ResourceBinding{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID},
	})
	if err != nil {
		return application.VerifiedCandidate{}, err
	}
	encoded, err := json.Marshal(resolved.Instance.Configuration)
	if err != nil {
		return application.VerifiedCandidate{}, err
	}
	capabilities := append([]string(nil), match.Capability.Required...)
	if match.Candidate != nil && len(inputs) == 0 {
		if match.Candidate.Digest() != proposal.CandidateDigest {
			return application.VerifiedCandidate{}, application.ErrApprovalStale
		}
		return application.VerifiedCandidate{
			DefinitionVersion: resolved.Instance.DefinitionVersion, Configuration: resolved.Instance.Configuration,
			CandidateDigest: match.Candidate.Digest(), InventoryWriteRevision: snapshot.WriteRevision,
		}, nil
	}
	candidatePayload, err := json.Marshal(struct {
		Resource     integration.ResourceIdentity `json:"resource"`
		DefinitionID string                       `json:"definition_id"`
		Version      string                       `json:"definition_version"`
		Capabilities []string                     `json:"capabilities"`
		Settings     json.RawMessage              `json:"settings"`
	}{
		Resource:     integration.ResourceIdentity{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID},
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
		DefinitionVersion: resolved.Instance.DefinitionVersion, Configuration: resolved.Instance.Configuration,
		CandidateDigest: candidateDigest, InventoryWriteRevision: snapshot.WriteRevision,
	}, nil
}

var _ application.CandidateVerifier = runtimeApprovalVerifier{}
