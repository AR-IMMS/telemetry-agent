package reconciliation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// SQLiteAttempts adapts shared SQLite state storage to reconciliation-owned attempt semantics.
type SQLiteAttempts struct {
	store agentstate.SQLiteStore
	now   func() time.Time
}

// NewSQLiteAttempts creates the durable attempt adapter at the Agent state database path.
func NewSQLiteAttempts(path string) *SQLiteAttempts {
	store, _ := agentstate.NewStore(path).(agentstate.SQLiteStore)
	return NewSQLiteAttemptsFromStore(store)
}

// NewSQLiteAttemptsFromStore reuses the resolved SQLite state adapter, including explicit legacy-path migration.
func NewSQLiteAttemptsFromStore(store agentstate.SQLiteStore) *SQLiteAttempts {
	return &SQLiteAttempts{store: store, now: time.Now}
}

func (s *SQLiteAttempts) BeginAttempt(target Target) (Attempt, error) {
	key, err := targetKey(target)
	if err != nil {
		return Attempt{}, err
	}
	started, err := s.store.BeginReconciliationAttempt(agentstate.ReconciliationTarget{Key: key, DesiredRevision: target.DesiredRevision, DesiredGeneration: target.DesiredGeneration, BaselineFingerprint: target.BaselineFingerprint}, s.now(), MaxAttempts)
	if err != nil {
		return Attempt{}, err
	}
	return Attempt{ID: started.ID, Target: target, Epoch: started.Epoch, Number: started.Number, StartedAt: s.now()}, nil
}

// RecordCandidate persists the compiled candidate identity before M2 admission.
func (s *SQLiteAttempts) RecordCandidate(attempt Attempt, digest string) error {
	key, err := targetKey(attempt.Target)
	if err != nil {
		return err
	}
	return s.store.RecordReconciliationCandidate(agentstate.ReconciliationAttempt{ID: attempt.ID, Target: agentstate.ReconciliationTarget{Key: key}, Epoch: attempt.Epoch}, digest, s.now())
}

func (s *SQLiteAttempts) FinishAttempt(attempt Attempt, outcome Outcome) error {
	key, err := targetKey(attempt.Target)
	if err != nil {
		return err
	}
	retryAt := ""
	if !outcome.RetryAt.IsZero() {
		retryAt = outcome.RetryAt.UTC().Format(time.RFC3339Nano)
	}
	return s.store.FinishReconciliationAttempt(agentstate.ReconciliationAttempt{ID: attempt.ID, Target: agentstate.ReconciliationTarget{Key: key}, Epoch: attempt.Epoch}, agentstate.ReconciliationOutcome{Status: string(outcome.Status), Stage: outcome.Stage, Code: outcome.Code, Class: string(outcome.Class), CandidateDigest: outcome.CandidateDigest, LifecycleGeneration: outcome.Generation, SafeDetail: outcome.SafeDetail, RetryAt: retryAt}, s.now())
}

// RecordRecoveryFailure links failed M2 startup recovery to the owning reconciliation attempt.
func (s *SQLiteAttempts) RecordRecoveryFailure(state agentstate.State) error {
	return s.store.RecordCandidateRecoveryFailure(state, s.now())
}

func targetKey(target Target) (string, error) {
	encoded, err := json.Marshal(target)
	if err != nil {
		return "", fmt.Errorf("encode reconciliation target: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// CandidateOutcomes exposes terminal and in-flight evidence only from the M2 activation journal.
type CandidateOutcomes struct{ Store agentstate.SQLiteStore }

func (r CandidateOutcomes) Lookup(revision uint64, fingerprint, digest string) (agentstate.CandidateMapping, uint64, bool, error) {
	return r.Store.CandidateOutcome(revision, fingerprint, digest)
}
