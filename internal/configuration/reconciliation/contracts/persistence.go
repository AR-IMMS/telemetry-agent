package contracts

import "time"

// TargetEvidence is the latest durable reconciliation state for one target.
// TargetKey is the stable persistence key used by the explicit retry command.
type TargetEvidence struct {
	Target          Target
	TargetKey       string
	Epoch           uint64
	Attempts        uint64
	Status          Status
	Stage           string
	Code            string
	Class           FailureClass
	CandidateDigest string
	Generation      uint64
	SafeDetail      string
	RetryAt         string
	UpdatedAt       string
}

// AttemptRecord is one safe, bounded row exposed by local reconciliation history.
type AttemptRecord struct {
	ID              int64
	TargetKey       string
	Epoch           uint64
	Number          uint64
	Status          string
	StartedAt       string
	UpdatedAt       string
	Stage           string
	Code            string
	Class           string
	CandidateDigest string
	Generation      uint64
	SafeDetail      string
	RetryAt         string
}

// RecoveryFailure identifies a Runtime candidate whose LKG recovery failed.
// Configuration uses this evidence to update the corresponding attempt and target
// within one SQLite transaction; it does not contain Runtime's persisted model.
type RecoveryFailure struct {
	DesiredRevision     uint64
	BaselineFingerprint string
	CandidateDigest     string
	Generation          uint64
}

// AttemptRepository persists the lifecycle of one reconciliation attempt.
type AttemptRepository interface {
	BeginAttempt(Target) (Attempt, error)
	RecordCandidate(Attempt, string) error
	FinishAttempt(Attempt, Outcome) error
}

// CandidateOutcomeReader reads authoritative Runtime journal evidence.
type CandidateOutcomeReader interface {
	Lookup(uint64, string, string) (CandidateOutcome, uint64, bool, error)
}

// RecoveryRecorder records failed Runtime recovery against Configuration evidence.
type RecoveryRecorder interface {
	RecordRecoveryFailure(RecoveryFailure) error
}

// Diagnostics exposes bounded local convergence evidence and explicit retry.
type Diagnostics interface {
	Evidence() (TargetEvidence, error)
	History(int) ([]AttemptRecord, error)
	RetryTarget(string, time.Time) error
}

// Repository is the Configuration-owned persistence seam used by the controller,
// lifecycle recovery adapter, and local diagnostics command.
type Repository interface {
	AttemptRepository
	CandidateOutcomeReader
	RecoveryRecorder
	Diagnostics
}
