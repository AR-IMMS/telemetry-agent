package discovery

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var proposalVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var proposalIdentityPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ValidateProposal checks proposal identity, bounds, status and safe payloads.
func ValidateProposal(proposal Proposal) error {
	wantID, err := StableProposalID(proposal.Identity)
	if err != nil || proposal.ID != wantID || !validProposalStatus(proposal.Status) {
		return fmt.Errorf("%w: invalid identity, ID, or status", ErrInvalidProposal)
	}
	if proposal.Revision == 0 || !proposalVersionPattern.MatchString(proposal.DefinitionVersion) || !proposalIdentityPattern.MatchString(proposal.Identity.IntegrationID) || len(proposal.Evidence) > 128 || len(proposal.RequiredInputs) > 64 || len(proposal.Diagnostics) > 64 {
		return fmt.Errorf("%w: invalid revision or field limits", ErrInvalidProposal)
	}
	if len(proposal.CandidateConfiguration) > maxProposalPayloadBytes || len(proposal.CapabilityEvidence) > 16*1024 || !validOptionalJSON(proposal.CandidateConfiguration) || !validOptionalJSON(proposal.CapabilityEvidence) {
		return fmt.Errorf("%w: invalid or oversized candidate evidence", ErrInvalidProposal)
	}
	if containsSecretJSON(proposal.CandidateConfiguration) || containsSecretJSON(proposal.CapabilityEvidence) {
		return fmt.Errorf("%w: secret-bearing proposal payload", ErrInvalidProposal)
	}
	if proposal.Status == ProposalPending && (proposal.CandidateDigest == "" || len(proposal.CandidateConfiguration) == 0 || proposal.ResourceFingerprint == "") {
		return fmt.Errorf("%w: pending proposal requires a candidate and resource fingerprint", ErrInvalidProposal)
	}
	if proposal.CandidateDigest != "" && !validProposalDigest(proposal.CandidateDigest) || proposal.ApprovedCandidateDigest != "" && !validProposalDigest(proposal.ApprovedCandidateDigest) || proposal.ResourceFingerprint != "" && !validProposalDigest(proposal.ResourceFingerprint) {
		return fmt.Errorf("%w: invalid SHA-256 digest", ErrInvalidProposal)
	}
	if proposal.Status == ProposalPending && len(proposal.CapabilityEvidence) == 0 {
		return fmt.Errorf("%w: pending proposal requires capability evidence", ErrInvalidProposal)
	}
	if proposal.ApprovedInstanceID != "" && (!proposalIdentityPattern.MatchString(proposal.ApprovedInstanceID) || proposal.ApprovedInstanceRevision == 0) ||
		proposal.ApprovedInstanceRevision > 0 && proposal.ApprovedInstanceID == "" ||
		proposal.ApprovedCandidateDigest != "" && !validProposalDigest(proposal.ApprovedCandidateDigest) {
		return fmt.Errorf("%w: invalid approved instance evidence", ErrInvalidProposal)
	}
	if proposal.Status == ProposalApproved && (proposal.ApprovedInstanceID == "" || proposal.ApprovedInstanceRevision == 0 || proposal.ApprovedCandidateDigest == "") {
		return fmt.Errorf("%w: approved proposal requires durable instance and candidate evidence", ErrInvalidProposal)
	}
	for _, evidence := range proposal.Evidence {
		if len(evidence.Code) == 0 || len(evidence.Code) > 128 || len(evidence.Source) > 256 || len(evidence.Value) > 1024 || hasControlText(evidence.Code) || hasControlText(evidence.Source) || hasControlText(evidence.Value) {
			return fmt.Errorf("%w: invalid evidence limits", ErrInvalidProposal)
		}
	}
	for _, input := range proposal.RequiredInputs {
		if input.Name == "" || len(input.Name) > 64 || len(input.Reason) > 256 || hasControlText(input.Name) || hasControlText(input.Reason) {
			return fmt.Errorf("%w: invalid required input", ErrInvalidProposal)
		}
	}
	for _, diagnostic := range proposal.Diagnostics {
		if !safeIdentifier.MatchString(diagnostic) || len(diagnostic) > 128 {
			return fmt.Errorf("%w: invalid diagnostic code", ErrInvalidProposal)
		}
	}
	if len(proposal.CandidateConfiguration) > 0 && !jsonObject(proposal.CandidateConfiguration) {
		return fmt.Errorf("%w: candidate configuration must be a JSON object", ErrInvalidProposal)
	}
	return nil
}

func validProposalStatus(status ProposalStatus) bool {
	switch status {
	case ProposalPending, ProposalNeedsInput, ProposalDismissed, ProposalApproved, ProposalStale:
		return true
	default:
		return false
	}
}

func validProposalMatchStatus(status string) bool {
	return status == ProposalMatchMatched || status == ProposalMatchAmbiguous || status == ProposalMatchStale || status == "no_match"
}

func validProposalResolutionStatus(status string) bool {
	return status == ProposalResolutionReady || status == ProposalResolutionNeedsInput || status == ProposalResolutionInvalid
}

func validProposalCapabilityStatus(status string) bool {
	return status == ProposalCapabilitySupported || status == ProposalCapabilityUnsupported || status == ProposalCapabilityUnverified || status == ProposalCapabilityNotAssessed
}

func validProposalDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validOptionalJSON(raw json.RawMessage) bool {
	return len(raw) == 0 || len(raw) <= maxProposalPayloadBytes && json.Valid(raw)
}

func jsonObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func containsSecretJSON(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	var walk func(any) bool
	walk = func(value any) bool {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				key = strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
				if strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "token") ||
					strings.Contains(key, "credential") || strings.Contains(key, "apikey") || strings.Contains(key, "authorization") ||
					strings.Contains(key, "privatekey") || walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

func hasControlText(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
