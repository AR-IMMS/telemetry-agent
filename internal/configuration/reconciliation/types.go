package reconciliation

import "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"

// MaxAttempts is the durable per-target automatic attempt budget.
const MaxAttempts = contracts.MaxAttempts

// Status is the operator-facing desired-to-applied convergence state.
type Status = contracts.Status

const (
	StatusPending        = contracts.StatusPending
	StatusApplied        = contracts.StatusApplied
	StatusBlocked        = contracts.StatusBlocked
	StatusNeedsAttention = contracts.StatusNeedsAttention
)

// FailureClass controls whether reconciliation blocks or retries a typed failure.
type FailureClass = contracts.FailureClass

const (
	FailurePermanent  = contracts.FailurePermanent
	FailureTransient  = contracts.FailureTransient
	FailureActivation = contracts.FailureActivation
)

// Target identifies the immutable desired and clean-baseline revisions processed by reconciliation.
type Target = contracts.Target

// Attempt identifies one durably charged reconciliation execution.
type Attempt = contracts.Attempt

// Outcome is safe structured diagnostic evidence for one attempt.
type Outcome = contracts.Outcome

// CandidateOutcome contains only the Runtime journal evidence needed for convergence decisions.
type CandidateOutcome = contracts.CandidateOutcome

var (
	// ErrAttemptBlocked indicates that the current desired target is permanently blocked.
	ErrAttemptBlocked = contracts.ErrAttemptBlocked
	// ErrAttemptBudgetExhausted indicates that the durable automatic retry budget is spent.
	ErrAttemptBudgetExhausted = contracts.ErrAttemptBudgetExhausted
	// ErrAttemptBackoff indicates that the persisted retry time has not elapsed.
	ErrAttemptBackoff = contracts.ErrAttemptBackoff
	// ErrTargetAlreadyApplied indicates that Runtime has authoritative applied evidence for this target.
	ErrTargetAlreadyApplied = contracts.ErrTargetAlreadyApplied
	// ErrAwaitingLifecycle indicates that Runtime still owns a pending candidate lifecycle operation.
	ErrAwaitingLifecycle = contracts.ErrAwaitingLifecycle
)
