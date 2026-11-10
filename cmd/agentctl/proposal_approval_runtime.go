package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

type runtimeProposalRefresher struct {
	inventory discovery.InventoryRepository
	manager   discovery.ProposalManager
	matcher   integration.Matcher
}

func newRuntimeProposalRefresher(statePath string) (runtimeProposalRefresher, error) {
	store := agentstate.NewSQLiteStore(statePath)
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return runtimeProposalRefresher{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor integration.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = catalogCapabilityAssessor{catalog: catalog, platform: platform.OS + "/" + platform.Architecture}
	}
	return runtimeProposalRefresher{
		inventory: store.InventoryRepository(), manager: discovery.NewProposalManager(store.ProposalRepository()),
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
	report, err := r.matcher.Match(projectInventory(snapshot, time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("match inventory for proposal refresh: %w", err)
	}
	observations := make([]discovery.ProposalObservation, 0, len(report.Results))
	for _, result := range report.Results {
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

func proposalObservationFromMatch(result integration.MatchResult, inventoryRevision uint64) (discovery.ProposalObservation, error) {
	observation := discovery.ProposalObservation{
		Identity: discovery.ProposalIdentity{
			Provider: result.Resource.Provider, ProviderInstance: result.Resource.ProviderInstance,
			ResourceID: result.Resource.ResourceID, IntegrationID: result.DefinitionID,
		},
		DefinitionVersion: result.DefinitionVersion, MatchStatus: string(result.MatchStatus),
		ResolutionStatus: string(result.ResolutionStatus), CapabilityStatus: string(result.CapabilityStatus),
		ResourceFingerprint: result.ResourceFingerprint, Evidence: make([]discovery.ProposalEvidence, 0, len(result.Evidence)),
		RequiredInputs: make([]discovery.ProposalInput, 0, len(result.RequiredInputs)), InventoryRevision: inventoryRevision,
	}
	if result.Candidate != nil {
		observation.CandidateConfiguration = result.Candidate.Configuration()
		observation.CandidateDigest = result.Candidate.Digest()
	}
	capabilityEvidence, err := json.Marshal(result.Capability)
	if err != nil {
		return discovery.ProposalObservation{}, fmt.Errorf("encode capability evidence: %w", err)
	}
	observation.CapabilityEvidence = capabilityEvidence
	for _, evidence := range result.Evidence {
		observation.Evidence = append(observation.Evidence, discovery.ProposalEvidence{Code: evidence.Code, Source: evidence.Source, Value: evidence.Value})
	}
	for _, input := range result.RequiredInputs {
		observation.RequiredInputs = append(observation.RequiredInputs, discovery.ProposalInput{Name: input.Name, Reason: input.Reason})
	}
	for _, diagnostic := range result.Diagnostics {
		observation.Diagnostics = append(observation.Diagnostics, diagnostic.Code)
	}
	return observation, nil
}

type runtimeApprovalVerifier struct {
	inventory discovery.InventoryRepository
	matcher   integration.Matcher
	resolver  integration.Resolver
}

func newRuntimeApprovalService(statePath string) (configuration.ApprovalService, error) {
	store := agentstate.NewSQLiteStore(statePath)
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return configuration.ApprovalService{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor integration.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = catalogCapabilityAssessor{catalog: catalog, platform: platform.OS + "/" + platform.Architecture}
	}
	matcher := integration.NewMatcher(registry, assessor)
	verifier := runtimeApprovalVerifier{
		inventory: store.InventoryRepository(), matcher: matcher, resolver: integration.NewResolver(registry),
	}
	return configuration.NewApprovalService(store.ProposalRepository(), verifier, store.ApprovalRepository()), nil
}

func (v runtimeApprovalVerifier) Verify(ctx context.Context, proposal discovery.Proposal, inputs map[string]any) (configuration.VerifiedCandidate, error) {
	if err := ctx.Err(); err != nil {
		return configuration.VerifiedCandidate{}, err
	}
	snapshot, err := v.inventory.Snapshot()
	if err != nil {
		return configuration.VerifiedCandidate{}, err
	}
	report, err := v.matcher.Match(projectInventory(snapshot, time.Now().UTC()))
	if err != nil {
		return configuration.VerifiedCandidate{}, err
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
		return configuration.VerifiedCandidate{}, configuration.ErrApprovalStale
	}
	settings := map[string]any{}
	if match.Candidate != nil {
		if err := json.Unmarshal(match.Candidate.Configuration(), &settings); err != nil {
			return configuration.VerifiedCandidate{}, err
		}
	}
	allowedInputs := make(map[string]struct{}, len(proposal.RequiredInputs))
	for _, input := range proposal.RequiredInputs {
		allowedInputs[input.Name] = struct{}{}
	}
	for name, value := range inputs {
		if _, ok := allowedInputs[name]; !ok {
			return configuration.VerifiedCandidate{}, configuration.ErrApprovalStale
		}
		settings[name] = value
	}
	if match.ResolutionStatus == integration.ResolutionNeedsInput && len(inputs) == 0 {
		return configuration.VerifiedCandidate{}, configuration.ErrApprovalStale
	}
	resolved, err := v.resolver.Resolve(integration.IntegrationInstance{
		ID: "approval-validation", DefinitionID: match.DefinitionID, DefinitionVersion: match.DefinitionVersion,
		Configuration: settings, Enabled: true, Source: integration.SourceDiscovery,
		Binding: &integration.ResourceBinding{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID},
	})
	if err != nil {
		return configuration.VerifiedCandidate{}, err
	}
	encoded, err := json.Marshal(resolved.Instance.Configuration)
	if err != nil {
		return configuration.VerifiedCandidate{}, err
	}
	capabilities := append([]string(nil), match.Capability.Required...)
	if match.Candidate != nil && len(inputs) == 0 {
		if match.Candidate.Digest() != proposal.CandidateDigest {
			return configuration.VerifiedCandidate{}, configuration.ErrApprovalStale
		}
		return configuration.VerifiedCandidate{
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
		return configuration.VerifiedCandidate{}, err
	}
	digest := sha256.Sum256(candidatePayload)
	candidateDigest := "sha256:" + hex.EncodeToString(digest[:])
	if proposal.CandidateDigest != "" && candidateDigest != proposal.CandidateDigest {
		return configuration.VerifiedCandidate{}, configuration.ErrApprovalStale
	}
	return configuration.VerifiedCandidate{
		DefinitionVersion: resolved.Instance.DefinitionVersion, Configuration: resolved.Instance.Configuration,
		CandidateDigest: candidateDigest, InventoryWriteRevision: snapshot.WriteRevision,
	}, nil
}

var _ configuration.CandidateVerifier = runtimeApprovalVerifier{}
