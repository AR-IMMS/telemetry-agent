package proposals

import application "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"

// ValidateProposal preserves the legacy validation entry point while the domain package owns invariants.
func ValidateProposal(proposal Proposal) error { return application.ValidateProposal(proposal) }
