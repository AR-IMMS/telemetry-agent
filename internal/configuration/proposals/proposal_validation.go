package proposals

import (
	proposaldomain "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/domain"
)

// ValidateProposal preserves the legacy validation entry point while the domain package owns invariants.
func ValidateProposal(proposal Proposal) error { return proposaldomain.ValidateProposal(proposal) }

func validProposalMatchStatus(status string) bool {
	return proposaldomain.ValidProposalMatchStatus(status)
}

func validProposalResolutionStatus(status string) bool {
	return proposaldomain.ValidProposalResolutionStatus(status)
}

func validProposalCapabilityStatus(status string) bool {
	return proposaldomain.ValidProposalCapabilityStatus(status)
}
