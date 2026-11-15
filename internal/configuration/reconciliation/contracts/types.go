package contracts

import (
	"errors"
	"time"
)

// MaxAttempts is the durable per-target automatic attempt budget.
const MaxAttempts = 5

// Status is the operator-facing desired-to-applied convergence state.
type Status string

const (
	StatusPending        Status = "Pending"
	StatusApplied        Status = "Applied"
	StatusBlocked        Status = "Blocked"
	StatusNeedsAttention Status = "Needs Attention"
)

// FailureClass controls whether reconciliation blocks or retries a typed failure.
type FailureClass string

const (
	FailurePermanent  FailureClass = "permanent"
	FailureTransient  FailureClass = "transient"
	FailureActivation FailureClass = "activation"
)

// Target identifies one desired Integration revision and clean-baseline target.
type Target struct {
	DesiredRevision     uint64
	DesiredGeneration   uint64
	BaselineFingerprint string
}

// Attempt identifies one durably charged reconciliation execution.
type Attempt struct {
	ID        int64
	Epoch     uint64
	Target    Target
	Number    uint64
	StartedAt time.Time
}

// Outcome is safe structured diagnostic evidence for one attempt.
type Outcome struct {
	Status          Status
	Stage           string
	Code            string
	Class           FailureClass
	CandidateDigest string
	Generation      uint64
	SafeDetail      string
	RetryAt         time.Time
}

// CandidateOutcome is the Runtime journal evidence needed to decide convergence.
// It intentionally omits Runtime's private activation-journal representation.
type CandidateOutcome struct {
	Outcome        string
	Stage          string
	RecoveryResult string
}

var (
	// ErrAttemptBlocked indicates that the current desired target is permanently blocked.
	ErrAttemptBlocked = errors.New("reconciliation target is blocked")
	// ErrAttemptBudgetExhausted indicates that the durable automatic retry budget is spent.
	ErrAttemptBudgetExhausted = errors.New("reconciliation attempt budget exhausted")
	// ErrAttemptBackoff indicates that the persisted retry time has not elapsed.
	ErrAttemptBackoff = errors.New("reconciliation retry backoff has not elapsed")
	// ErrTargetAlreadyApplied indicates that Runtime has authoritative applied evidence for this target.
	ErrTargetAlreadyApplied = errors.New("reconciliation target is already applied")
	// ErrAwaitingLifecycle indicates that Runtime still owns a pending candidate lifecycle operation.
	ErrAwaitingLifecycle = errors.New("reconciliation target awaits Runtime lifecycle completion")
)
