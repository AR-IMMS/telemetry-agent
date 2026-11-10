package agentstate

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestReconciliationAttemptBudgetAndRestartAreDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	target := ReconciliationTarget{Key: "target", DesiredRevision: 12, DesiredGeneration: 37, BaselineFingerprint: "baseline"}
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	first, err := store.BeginReconciliationAttempt(target, now, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishReconciliationAttempt(first, ReconciliationOutcome{Status: "Pending", Stage: "compile", Code: "timeout", Class: "transient", RetryAt: now.Add(time.Second).Format(time.RFC3339Nano)}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginReconciliationAttempt(target, now, 5); !errors.Is(err, ErrReconciliationBackoff) {
		t.Fatalf("attempt during backoff error = %v", err)
	}
	second, err := store.BeginReconciliationAttempt(target, now.Add(2*time.Second), 5)
	if err != nil {
		t.Fatal(err)
	}
	if second.Number != 2 {
		t.Fatalf("attempt number = %d, want 2 after restart", second.Number)
	}
	if err := store.FinishReconciliationAttempt(second, ReconciliationOutcome{Status: "Needs Attention", Stage: "activation", Code: "failed", Class: "activation"}, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	evidence, err := store.ReconciliationEvidence()
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Attempts != 2 || evidence.Status != "Needs Attention" || evidence.Target.DesiredRevision != 12 {
		t.Fatalf("evidence = %+v", evidence)
	}
	history, err := store.ReconciliationHistory(10)
	if err != nil || len(history) != 2 {
		t.Fatalf("history = %d, %v", len(history), err)
	}
}

func TestInterruptedAttemptConsumesBudgetAndExplicitRetryStartsNewEpoch(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	target := ReconciliationTarget{Key: "target", DesiredRevision: 1, DesiredGeneration: 2, BaselineFingerprint: "base"}
	first, err := store.BeginReconciliationAttempt(target, time.Now(), 5)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.BeginReconciliationAttempt(target, time.Now().Add(time.Second), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RetryReconciliationTarget("target", time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("retry during active attempt error = %v, want no eligible target", err)
	}
	if second.Number != 2 {
		t.Fatalf("restart attempt number = %d, want 2", second.Number)
	}
	history, err := store.ReconciliationHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[1].Status != "interrupted" {
		t.Fatalf("history after interrupted attempt = %+v (first id %d)", history, first.ID)
	}
	if err := store.FinishReconciliationAttempt(second, ReconciliationOutcome{Status: "Blocked", Stage: "resolve", Code: "invalid", Class: "permanent"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginReconciliationAttempt(target, time.Now().Add(time.Minute), 5); !errors.Is(err, ErrReconciliationBlocked) {
		t.Fatalf("blocked target attempt error = %v", err)
	}
	if err := store.RetryReconciliationTarget("target", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	third, err := store.BeginReconciliationAttempt(target, time.Now().Add(time.Minute), 5)
	if err != nil {
		t.Fatal(err)
	}
	if third.Epoch != 1 || third.Number != 1 {
		t.Fatalf("explicit retry = epoch %d attempt %d", third.Epoch, third.Number)
	}
}

func TestReconciliationEnforcesFiveTotalAttempts(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	target := ReconciliationTarget{Key: "five", DesiredRevision: 2, DesiredGeneration: 3, BaselineFingerprint: "fp"}
	now := time.Now()
	for i := uint64(1); i <= 5; i++ {
		attempt, err := store.BeginReconciliationAttempt(target, now.Add(time.Duration(i)*time.Minute), 5)
		if err != nil {
			t.Fatalf("attempt %d start: %v", i, err)
		}
		if attempt.Number != i {
			t.Fatalf("attempt number=%d want %d", attempt.Number, i)
		}
		if err := store.FinishReconciliationAttempt(attempt, ReconciliationOutcome{Status: "Needs Attention", Stage: "activation", Code: "failed", Class: "activation"}, now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.BeginReconciliationAttempt(target, now.Add(10*time.Minute), 5); !errors.Is(err, ErrReconciliationExhausted) {
		t.Fatalf("sixth attempt error=%v", err)
	}
}

func TestCandidateOutcomeUsesM2TerminalJournalEvidence(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(s *State) error { s.DesiredGeneration = 4; return nil }); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(0, 4, "baseline", "digest")
	admission, err := store.AdmitCandidate(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := store.CandidateOutcome(0, "baseline", "digest"); err != nil || found {
		t.Fatalf("pre-activation outcome found=%t err=%v", found, err)
	}
	if _, err := store.Update(func(s *State) error { return s.MarkCandidateGenerationActivated(admission.Generation) }); err != nil {
		t.Fatal(err)
	}
	mapping, generation, found, err := store.CandidateOutcome(0, "baseline", "digest")
	if err != nil || !found || mapping.Outcome != "activated" || generation != admission.Generation {
		t.Fatalf("activation evidence=%+v gen=%d found=%t err=%v", mapping, generation, found, err)
	}
	if err := store.MarkCandidateGenerationApplied(admission.Generation, ""); err != nil {
		t.Fatal(err)
	}
	mapping, generation, found, err = store.CandidateOutcome(0, "baseline", "digest")
	if err != nil || !found || mapping.Outcome != "applied" || generation != admission.Generation {
		t.Fatalf("terminal evidence=%+v gen=%d found=%t err=%v", mapping, generation, found, err)
	}
}

func TestAwaitingReconciliationTargetRecoversAppliedStatusFromM2Journal(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error { state.DesiredGeneration = 4; return nil }); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(0, 4, "baseline", "candidate")
	admission, err := store.AdmitCandidate(request)
	if err != nil {
		t.Fatal(err)
	}
	target := ReconciliationTarget{Key: "awaiting", DesiredRevision: 0, DesiredGeneration: 4, BaselineFingerprint: "baseline"}
	attempt, err := store.BeginReconciliationAttempt(target, time.Now(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishReconciliationAttempt(attempt, ReconciliationOutcome{Status: "Pending", Stage: "activation", Code: "awaiting_m2_terminal_outcome", CandidateDigest: "candidate", LifecycleGeneration: admission.Generation}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkCandidateGenerationActivated(admission.Generation) }); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCandidateGenerationApplied(admission.Generation, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginReconciliationAttempt(target, time.Now().Add(time.Second), 5); !errors.Is(err, ErrReconciliationApplied) {
		t.Fatalf("reopened target error = %v, want authoritative applied evidence", err)
	}
	evidence, err := store.ReconciliationEvidence()
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != "Applied" || evidence.LifecycleGeneration != admission.Generation {
		t.Fatalf("recovered reconciliation evidence = %+v", evidence)
	}
}

func TestCandidateRecoveryFailureIsPersistedAgainstPendingAttempt(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error {
		state.DesiredGeneration = 37
		state.AppliedGeneration = 36
		state.ActivatedGeneration = 37
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.openAndExec(`UPDATE integration_desired_state SET desired_set_revision=12 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(12, 37, "baseline-12", "candidate-12")
	if _, err := store.AdmitCandidate(request); err != nil {
		t.Fatal(err)
	}
	target := ReconciliationTarget{Key: "target-12", DesiredRevision: 12, DesiredGeneration: 36, BaselineFingerprint: "baseline-12"}
	attempt, err := store.BeginReconciliationAttempt(target, time.Now(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordReconciliationCandidate(attempt, "candidate-12", time.Now()); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCandidateRecoveryFailure(state, time.Now()); err != nil {
		t.Fatal(err)
	}
	history, err := store.ReconciliationHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != attempt.ID || history[0].Status != "Needs Attention" || history[0].Code != "lkg_recovery_failed" {
		t.Fatalf("recovery failure history = %+v", history)
	}
	evidence, err := store.ReconciliationEvidence()
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != "Needs Attention" || evidence.Stage != "recovery" || evidence.LifecycleGeneration != state.PendingActivation.Generation {
		t.Fatalf("recovery failure evidence = %+v", evidence)
	}
}

func TestReconciliationDiagnosticsMigrationPreservesM2AndCandidateHistory(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	want := State{DesiredGeneration: 37, ActivatedGeneration: 36, AppliedGeneration: 35, Dependencies: map[string]DependencyState{"node-exporter": {Enabled: true}}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdmitCandidate(candidateAdmissionForTest(0, 37, "baseline", "digest")); err != nil {
		t.Fatal(err)
	}
	beforeMigration, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO activation_journal(generation,outcome,updated_at,desired_set_revision,candidate_digest,baseline_fingerprint,effective_config_digest,collector_version,manifest_version,platform,stage,recovery_result,history_json) VALUES(35,'applied',CURRENT_TIMESTAMP,0,'digest','baseline','effective','0.160.0','manifest','windows/amd64','readiness','not_needed','[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=9`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE reconciliation_attempts`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE reconciliation_targets`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DesiredGeneration != beforeMigration.DesiredGeneration || got.ActivatedGeneration != beforeMigration.ActivatedGeneration || got.AppliedGeneration != beforeMigration.AppliedGeneration || !got.Dependencies["node-exporter"].Enabled || got.PendingActivation == nil {
		t.Fatalf("M2 state changed across diagnostics migration: %+v", got)
	}
	db, err = store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 12 {
		t.Fatalf("schema version=%d err=%v want 12", version, err)
	}
	var mappings int
	if err := db.QueryRow(`SELECT COUNT(*) FROM activation_journal WHERE candidate_digest='digest' AND outcome='applied'`).Scan(&mappings); err != nil || mappings != 1 {
		t.Fatalf("legacy candidate journal mappings=%d err=%v", mappings, err)
	}
}
