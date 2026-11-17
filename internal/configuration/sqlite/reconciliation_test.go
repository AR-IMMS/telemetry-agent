package sqlite_test

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func TestReconciliationRepositoryPersistsBudgetBackoffAndExplicitRetry(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	_, repository := newReconciliationRepository(t, &now)
	target := contracts.Target{DesiredRevision: 12, DesiredGeneration: 37, BaselineFingerprint: "baseline"}
	first, err := repository.BeginAttempt(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.FinishAttempt(first, contracts.Outcome{Status: contracts.StatusPending, Stage: "compile", Code: "timeout", Class: contracts.FailureTransient, RetryAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.BeginAttempt(target); !errors.Is(err, contracts.ErrAttemptBackoff) {
		t.Fatalf("attempt during backoff error = %v", err)
	}
	now = now.Add(2 * time.Second)
	second, err := repository.BeginAttempt(target)
	if err != nil {
		t.Fatal(err)
	}
	if second.Number != 2 {
		t.Fatalf("attempt number = %d, want 2", second.Number)
	}
	if err := repository.RecordCandidate(second, "candidate-digest"); err != nil {
		t.Fatal(err)
	}
	if err := repository.FinishAttempt(second, contracts.Outcome{Status: contracts.StatusNeedsAttention, Stage: "activation", Code: "failed", Class: contracts.FailureActivation, CandidateDigest: "candidate-digest"}); err != nil {
		t.Fatal(err)
	}
	evidence, err := repository.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Attempts != 2 || evidence.Status != contracts.StatusNeedsAttention || evidence.Target.DesiredRevision != 12 || evidence.CandidateDigest != "candidate-digest" {
		t.Fatalf("evidence = %+v", evidence)
	}
	history, err := repository.History(10)
	if err != nil || len(history) != 2 {
		t.Fatalf("history count=%d err=%v", len(history), err)
	}
	if err := repository.RetryTarget(evidence.TargetKey, now); err != nil {
		t.Fatal(err)
	}
	third, err := repository.BeginAttempt(target)
	if err != nil {
		t.Fatal(err)
	}
	if third.Epoch != 1 || third.Number != 1 {
		t.Fatalf("explicit retry = epoch %d attempt %d", third.Epoch, third.Number)
	}
}

func TestReconciliationRepositoryInterruptsAndBoundsAttempts(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	_, repository := newReconciliationRepository(t, &now)
	target := contracts.Target{DesiredRevision: 1, DesiredGeneration: 2, BaselineFingerprint: "baseline"}
	first, err := repository.BeginAttempt(target)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	second, err := repository.BeginAttempt(target)
	if err != nil {
		t.Fatal(err)
	}
	if second.Number != 2 {
		t.Fatalf("restarted attempt number = %d, want 2", second.Number)
	}
	history, err := repository.History(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[1].ID != first.ID || history[1].Status != "interrupted" {
		t.Fatalf("history after restart = %+v", history)
	}
	if err := repository.FinishAttempt(second, contracts.Outcome{Status: contracts.StatusBlocked, Stage: "resolve", Code: "invalid", Class: contracts.FailurePermanent}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.BeginAttempt(target); !errors.Is(err, contracts.ErrAttemptBlocked) {
		t.Fatalf("blocked target error = %v", err)
	}
	evidence, err := repository.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RetryTarget(evidence.TargetKey, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.BeginAttempt(target); err != nil {
		t.Fatalf("begin after explicit retry: %v", err)
	}
}

func TestReconciliationRepositoryEnforcesFiveAttemptBudget(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	_, repository := newReconciliationRepository(t, &now)
	target := contracts.Target{DesiredRevision: 2, DesiredGeneration: 3, BaselineFingerprint: "fingerprint"}
	for i := 1; i <= contracts.MaxAttempts; i++ {
		attempt, err := repository.BeginAttempt(target)
		if err != nil {
			t.Fatalf("begin attempt %d: %v", i, err)
		}
		if attempt.Number != uint64(i) {
			t.Fatalf("attempt number=%d want %d", attempt.Number, i)
		}
		if err := repository.FinishAttempt(attempt, contracts.Outcome{Status: contracts.StatusNeedsAttention, Stage: "activation", Code: "failed", Class: contracts.FailureActivation}); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
	}
	if _, err := repository.BeginAttempt(target); !errors.Is(err, contracts.ErrAttemptBudgetExhausted) {
		t.Fatalf("sixth attempt error = %v", err)
	}
}

func TestReconciliationRepositoryRecoversAppliedStatusFromRuntimeJournal(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	store, repository := newReconciliationRepository(t, &now)
	if _, err := store.Update(func(state *agentstate.State) error { state.DesiredGeneration = 4; return nil }); err != nil {
		t.Fatal(err)
	}
	admission, err := store.AdmitCandidate(candidateAdmission(0, 4, "baseline", "candidate"))
	if err != nil {
		t.Fatal(err)
	}
	target := contracts.Target{DesiredRevision: 0, DesiredGeneration: 4, BaselineFingerprint: "baseline"}
	attempt, err := repository.BeginAttempt(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.FinishAttempt(attempt, contracts.Outcome{Status: contracts.StatusPending, Stage: "activation", Code: "awaiting_m2_terminal_outcome", CandidateDigest: "candidate", Generation: admission.Generation}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *agentstate.State) error {
		return state.MarkCandidateGenerationActivated(admission.Generation)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCandidateGenerationApplied(admission.Generation, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.BeginAttempt(target); !errors.Is(err, contracts.ErrTargetAlreadyApplied) {
		t.Fatalf("reopened target error = %v", err)
	}
	evidence, err := repository.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != contracts.StatusApplied || evidence.Generation != admission.Generation {
		t.Fatalf("recovered evidence = %+v", evidence)
	}
}

func TestReconciliationRepositoryPersistsLKGRecoveryFailure(t *testing.T) {
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	store, repository := newReconciliationRepository(t, &now)
	if _, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 37
		state.AppliedGeneration = 36
		state.ActivatedGeneration = 37
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := setDesiredRevision(store, 12); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdmitCandidate(candidateAdmission(12, 37, "baseline-12", "candidate-12")); err != nil {
		t.Fatal(err)
	}
	target := contracts.Target{DesiredRevision: 12, DesiredGeneration: 36, BaselineFingerprint: "baseline-12"}
	attempt, err := repository.BeginAttempt(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordCandidate(attempt, "candidate-12"); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordRecoveryFailure(contracts.RecoveryFailure{DesiredRevision: 12, BaselineFingerprint: "baseline-12", CandidateDigest: "candidate-12", Generation: state.PendingActivation.Generation}); err != nil {
		t.Fatal(err)
	}
	history, err := repository.History(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != attempt.ID || history[0].Status != "Needs Attention" || history[0].Code != "lkg_recovery_failed" {
		t.Fatalf("recovery history = %+v", history)
	}
	evidence, err := repository.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != contracts.StatusNeedsAttention || evidence.Stage != "recovery" || evidence.Generation != state.PendingActivation.Generation {
		t.Fatalf("recovery evidence = %+v", evidence)
	}
}

func TestReconciliationDiagnosticsMigrationPreservesRuntimeAndCandidateHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := sqlitefixture.NewSQLiteStore(path)
	want := agentstate.State{DesiredGeneration: 37, ActivatedGeneration: 36, AppliedGeneration: 35, Dependencies: map[string]dependencydomain.DependencyState{"node-exporter": {Enabled: true}}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdmitCandidate(candidateAdmission(0, 37, "baseline", "digest")); err != nil {
		t.Fatal(err)
	}
	beforeMigration, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO activation_journal(generation,outcome,updated_at,desired_set_revision,candidate_digest,baseline_fingerprint,effective_config_digest,collector_version,manifest_version,platform,stage,recovery_result,history_json) VALUES(35,'applied',CURRENT_TIMESTAMP,0,'digest','baseline','effective','0.160.0','manifest','windows/amd64','readiness','not_needed','[]')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=9`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE reconciliation_attempts`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE reconciliation_targets`); err != nil {
		db.Close()
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
		t.Fatalf("Runtime state changed across diagnostics migration: %+v", got)
	}
	db, err = store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, mappings int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 14 {
		t.Fatalf("schema version=%d err=%v want 14", version, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM activation_journal WHERE candidate_digest='digest' AND outcome='applied'`).Scan(&mappings); err != nil || mappings != 1 {
		t.Fatalf("candidate journal mappings=%d err=%v", mappings, err)
	}
}

func newReconciliationRepository(t *testing.T, now *time.Time) (agentstate.SQLiteStore, contracts.Repository) {
	t.Helper()
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	repository := configurationsqlite.NewReconciliationRepositoryWithClock(store.OpenDatabase, &sync.Mutex{}, store, func() time.Time { return *now })
	return store, repository
}

func candidateAdmission(revision, generation uint64, baseline, digest string) agentstate.CandidateAdmission {
	return agentstate.CandidateAdmission{
		DesiredSetRevision: revision, ExpectedGeneration: generation,
		ExpectedFingerprint: baseline, VerifiedFingerprint: baseline, RecoveryPath: "recovery.yaml",
		Mapping: agentstate.CandidateMapping{
			DesiredSetRevision: revision, BaselineFingerprint: baseline, CandidateDigest: digest,
			EffectiveConfigDigest: "effective-" + digest, CollectorVersion: "0.160.0",
			ManifestVersion: "test", Platform: "windows/amd64",
		},
	}
}

func setDesiredRevision(store agentstate.SQLiteStore, revision uint64) error {
	db, err := store.OpenDatabase()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`UPDATE integration_desired_state SET desired_set_revision=? WHERE id=1`, revision)
	return err
}
