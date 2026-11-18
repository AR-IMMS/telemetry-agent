package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	matchingapp "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/application"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

// CandidateMatcher re-evaluates current resource evidence against supported monitoring definitions.
type CandidateMatcher interface {
	Match(matchingapp.MatchSnapshot) (matchingapp.MatchReport, error)
}

// CandidateVerificationService verifies that an approval still matches current inventory and profile rules.
type CandidateVerificationService struct {
	inventory InventorySnapshotReader
	matcher   CandidateMatcher
	resolver  definitionapp.Resolver
}

// NewCandidateVerificationService composes fresh inventory matching with profile resolution.
func NewCandidateVerificationService(inventory InventorySnapshotReader, matcher CandidateMatcher, resolver definitionapp.Resolver) CandidateVerificationService {
	return CandidateVerificationService{inventory: inventory, matcher: matcher, resolver: resolver}
}

// Verify rebuilds and validates the approved candidate from current inventory evidence.
func (s CandidateVerificationService) Verify(ctx context.Context, proposal Proposal, inputs map[string]any) (VerifiedCandidate, error) {
	if ctx == nil || s.inventory == nil || s.matcher == nil {
		return VerifiedCandidate{}, fmt.Errorf("verification context, inventory reader, and matcher are required")
	}
	if err := ctx.Err(); err != nil {
		return VerifiedCandidate{}, err
	}
	snapshot, err := s.inventory.SnapshotContext(ctx)
	if err != nil {
		return VerifiedCandidate{}, err
	}
	if err := ctx.Err(); err != nil {
		return VerifiedCandidate{}, err
	}
	report, err := s.matcher.Match(matchingapp.ProjectInventory(snapshot, time.Now().UTC()))
	if err != nil {
		return VerifiedCandidate{}, err
	}
	if err := ctx.Err(); err != nil {
		return VerifiedCandidate{}, err
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
		return VerifiedCandidate{}, ErrApprovalStale
	}
	settings := map[string]any{}
	if match.Candidate != nil {
		if err := json.Unmarshal(match.Candidate.Configuration(), &settings); err != nil {
			return VerifiedCandidate{}, err
		}
	}
	allowedInputs := make(map[string]struct{}, len(proposal.RequiredInputs))
	for _, input := range proposal.RequiredInputs {
		allowedInputs[input.Name] = struct{}{}
	}
	for name, value := range inputs {
		if _, ok := allowedInputs[name]; !ok {
			return VerifiedCandidate{}, ErrApprovalStale
		}
		settings[name] = value
	}
	if match.ResolutionStatus == matchingdomain.ResolutionNeedsInput && len(inputs) == 0 {
		return VerifiedCandidate{}, ErrApprovalStale
	}
	validationInstance := desiredstatedomain.Instance{
		ID: "approval-validation", DefinitionID: match.DefinitionID, DefinitionVersion: match.DefinitionVersion,
		Configuration: settings, Enabled: true, Source: desiredstatedomain.SourceDiscovery,
		Binding: &desiredstatedomain.ResourceBinding{
			Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID,
		},
	}
	resolved, err := ResolveApprovalSettings(s.resolver, validationInstance)
	if err != nil {
		return VerifiedCandidate{}, err
	}
	if err := ctx.Err(); err != nil {
		return VerifiedCandidate{}, err
	}
	encoded, err := json.Marshal(resolved.Settings)
	if err != nil {
		return VerifiedCandidate{}, err
	}
	capabilities := append([]string(nil), match.Capability.Required...)
	if match.Candidate != nil && len(inputs) == 0 {
		if match.Candidate.Digest() != proposal.CandidateDigest {
			return VerifiedCandidate{}, ErrApprovalStale
		}
		return VerifiedCandidate{
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
		return VerifiedCandidate{}, err
	}
	digest := sha256.Sum256(candidatePayload)
	candidateDigest := "sha256:" + hex.EncodeToString(digest[:])
	if proposal.CandidateDigest != "" && candidateDigest != proposal.CandidateDigest {
		return VerifiedCandidate{}, ErrApprovalStale
	}
	return VerifiedCandidate{
		DefinitionVersion: resolved.Version, Configuration: resolved.Settings,
		CandidateDigest: candidateDigest, InventoryWriteRevision: snapshot.WriteRevision,
	}, nil
}

// ResolveApprovalSettings preserves the established classification of profile validation errors.
func ResolveApprovalSettings(resolver definitionapp.Resolver, instance desiredstatedomain.Instance) (definitionapp.Resolved, error) {
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
