package agentstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	reconciliation "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
)

// SQLiteAttempts adapts the transitional SQLite store to Configuration reconciliation ports.
type SQLiteAttempts struct {
	store SQLiteStore
	now   func() time.Time
}

// NewSQLiteAttemptsFromStore reuses the resolved SQLite adapter, including explicit legacy-path migration.
func NewSQLiteAttemptsFromStore(store SQLiteStore) *SQLiteAttempts {
	return &SQLiteAttempts{store: store, now: time.Now}
}

func (s *SQLiteAttempts) BeginAttempt(target reconciliation.Target) (reconciliation.Attempt, error) {
	key, err := targetKey(target)
	if err != nil {
		return reconciliation.Attempt{}, err
	}
	started, err := s.store.BeginReconciliationAttempt(ReconciliationTarget{Key: key, DesiredRevision: target.DesiredRevision, DesiredGeneration: target.DesiredGeneration, BaselineFingerprint: target.BaselineFingerprint}, s.now(), reconciliation.MaxAttempts)
	if err != nil {
		return reconciliation.Attempt{}, translateReconciliationError(err)
	}
	return reconciliation.Attempt{ID: started.ID, Target: target, Epoch: started.Epoch, Number: started.Number, StartedAt: s.now()}, nil
}

// RecordCandidate persists the compiled candidate identity before M2 admission.
func (s *SQLiteAttempts) RecordCandidate(attempt reconciliation.Attempt, digest string) error {
	key, err := targetKey(attempt.Target)
	if err != nil {
		return err
	}
	return s.store.RecordReconciliationCandidate(ReconciliationAttempt{ID: attempt.ID, Target: ReconciliationTarget{Key: key}, Epoch: attempt.Epoch}, digest, s.now())
}

func (s *SQLiteAttempts) FinishAttempt(attempt reconciliation.Attempt, outcome reconciliation.Outcome) error {
	key, err := targetKey(attempt.Target)
	if err != nil {
		return err
	}
	retryAt := ""
	if !outcome.RetryAt.IsZero() {
		retryAt = outcome.RetryAt.UTC().Format(time.RFC3339Nano)
	}
	return s.store.FinishReconciliationAttempt(ReconciliationAttempt{ID: attempt.ID, Target: ReconciliationTarget{Key: key}, Epoch: attempt.Epoch}, ReconciliationOutcome{Status: string(outcome.Status), Stage: outcome.Stage, Code: outcome.Code, Class: string(outcome.Class), CandidateDigest: outcome.CandidateDigest, LifecycleGeneration: outcome.Generation, SafeDetail: outcome.SafeDetail, RetryAt: retryAt}, s.now())
}

// RecordRecoveryFailure links failed M2 startup recovery to the owning reconciliation attempt.
func (s *SQLiteAttempts) RecordRecoveryFailure(state State) error {
	return s.store.RecordCandidateRecoveryFailure(state, s.now())
}

func targetKey(target reconciliation.Target) (string, error) {
	encoded, err := json.Marshal(target)
	if err != nil {
		return "", fmt.Errorf("encode reconciliation target: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Lookup exposes the minimal terminal and in-flight evidence from the M2 journal.
func (s *SQLiteAttempts) Lookup(revision uint64, fingerprint, digest string) (reconciliation.CandidateOutcome, uint64, bool, error) {
	mapping, generation, found, err := s.store.CandidateOutcome(revision, fingerprint, digest)
	if err != nil || !found {
		return reconciliation.CandidateOutcome{}, generation, found, err
	}
	return reconciliation.CandidateOutcome{Outcome: mapping.Outcome, Stage: mapping.Stage, RecoveryResult: mapping.RecoveryResult}, generation, true, nil
}

func translateReconciliationError(err error) error {
	switch {
	case errors.Is(err, ErrReconciliationBlocked):
		return reconciliation.ErrAttemptBlocked
	case errors.Is(err, ErrReconciliationExhausted):
		return reconciliation.ErrAttemptBudgetExhausted
	case errors.Is(err, ErrReconciliationBackoff):
		return reconciliation.ErrAttemptBackoff
	case errors.Is(err, ErrReconciliationApplied):
		return reconciliation.ErrTargetAlreadyApplied
	case errors.Is(err, ErrReconciliationAwaitingLifecycle):
		return reconciliation.ErrAwaitingLifecycle
	default:
		return err
	}
}
