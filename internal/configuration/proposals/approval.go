package proposals

import application "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"

// ApprovalRequest identifies the exact proposal and desired-state snapshot reviewed by the operator.
type ApprovalRequest = application.ApprovalRequest

// VerifiedCandidate contains fresh, revalidated non-secret settings from current inventory evidence.
type VerifiedCandidate = application.VerifiedCandidate

// CandidateVerifier re-matches a proposal against fresh Inventory evidence before approval.
type CandidateVerifier = application.CandidateVerifier

// ApprovalCommit is the validated state transition persisted atomically by the repository.
type ApprovalCommit = application.ApprovalCommit

// ApprovalRepository atomically commits proposal outcome and desired instance state.
type ApprovalRepository = application.ApprovalRepository

// ApprovalService preserves the legacy import path for explicit proposal approval.
type ApprovalService = application.ApprovalService

// NewApprovalService creates the application use case with its repository and evidence ports.
func NewApprovalService(proposals ProposalRepository, verifier CandidateVerifier, repository ApprovalRepository) ApprovalService {
	return application.NewApprovalService(proposals, verifier, repository)
}

var (
	// ErrApprovalConflict indicates proposal or desired state changed during review.
	ErrApprovalConflict = application.ErrApprovalConflict
	// ErrApprovalNotEligible indicates the proposal cannot be approved in its current state.
	ErrApprovalNotEligible = application.ErrApprovalNotEligible
	// ErrApprovalStale indicates verified resource or candidate evidence changed.
	ErrApprovalStale = application.ErrApprovalStale
	// ErrApprovalBindingConflict indicates another desired Integration owns the resource.
	ErrApprovalBindingConflict = application.ErrApprovalBindingConflict
	// ErrApprovalDuplicate indicates an equivalent desired Integration already exists.
	ErrApprovalDuplicate = application.ErrApprovalDuplicate
)
