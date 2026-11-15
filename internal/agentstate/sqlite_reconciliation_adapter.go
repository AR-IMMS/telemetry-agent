package agentstate

import (
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
)

// SQLiteAttempts is a compatibility adapter from Runtime recovery state to the
// Configuration-owned reconciliation repository.
type SQLiteAttempts struct {
	repository contracts.Repository
}

// NewSQLiteAttemptsFromStore composes Configuration persistence with Runtime's
// candidate journal reader over the resolved shared SQLite store.
func NewSQLiteAttemptsFromStore(store SQLiteStore) *SQLiteAttempts {
	return &SQLiteAttempts{repository: store.ReconciliationRepository()}
}

func (s *SQLiteAttempts) BeginAttempt(target contracts.Target) (contracts.Attempt, error) {
	return s.repository.BeginAttempt(target)
}

// RecordCandidate persists the compiled candidate identity before Runtime admission.
func (s *SQLiteAttempts) RecordCandidate(attempt contracts.Attempt, digest string) error {
	return s.repository.RecordCandidate(attempt, digest)
}

func (s *SQLiteAttempts) FinishAttempt(attempt contracts.Attempt, outcome contracts.Outcome) error {
	return s.repository.FinishAttempt(attempt, outcome)
}

// RecordRecoveryFailure translates Runtime pending-candidate state to the small
// Configuration recovery-evidence contract.
func (s *SQLiteAttempts) RecordRecoveryFailure(state State) error {
	if state.PendingActivation == nil || state.PendingActivation.Candidate == nil {
		return fmt.Errorf("pending candidate recovery evidence is required")
	}
	pending := state.PendingActivation
	mapping := pending.Candidate
	return s.repository.RecordRecoveryFailure(contracts.RecoveryFailure{
		DesiredRevision:     mapping.DesiredSetRevision,
		BaselineFingerprint: mapping.BaselineFingerprint,
		CandidateDigest:     mapping.CandidateDigest,
		Generation:          pending.Generation,
	})
}

// Lookup exposes minimal terminal and in-flight evidence from Runtime's journal.
func (s *SQLiteAttempts) Lookup(revision uint64, fingerprint, digest string) (contracts.CandidateOutcome, uint64, bool, error) {
	return s.repository.Lookup(revision, fingerprint, digest)
}
