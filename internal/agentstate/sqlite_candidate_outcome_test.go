package agentstate

import (
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
)

func TestCandidateOutcomeReaderUsesRuntimeJournalAndAppliedGeneration(t *testing.T) {
	store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error { state.DesiredGeneration = 4; return nil }); err != nil {
		t.Fatal(err)
	}
	admission, err := store.AdmitCandidate(candidateAdmissionForTest(0, 4, "baseline", "digest"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := store.CandidateOutcome(0, "baseline", "digest"); err != nil || found {
		t.Fatalf("pre-activation outcome found=%t err=%v", found, err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkCandidateGenerationActivated(admission.Generation) }); err != nil {
		t.Fatal(err)
	}
	mapping, generation, found, err := store.CandidateOutcome(0, "baseline", "digest")
	if err != nil || !found || mapping.Outcome != "activated" || generation != admission.Generation {
		t.Fatalf("activation evidence=%+v generation=%d found=%t err=%v", mapping, generation, found, err)
	}
	if err := store.MarkCandidateGenerationApplied(admission.Generation, ""); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	outcome, generation, found, err := store.CandidateOutcomeTx(tx, 0, "baseline", "digest")
	if err != nil || !found || outcome.Outcome != "applied" || generation != admission.Generation {
		t.Fatalf("transactional applied evidence=%+v generation=%d found=%t err=%v", outcome, generation, found, err)
	}
	state, err := loadSQLiteState(tx)
	if err != nil || state.AppliedGeneration < generation {
		t.Fatalf("journal result lacks persisted AppliedGeneration=%d generation=%d err=%v", state.AppliedGeneration, generation, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if outcome != (contracts.CandidateOutcome{Outcome: "applied", Stage: "readiness", RecoveryResult: "not_needed"}) {
		t.Fatalf("candidate outcome = %+v", outcome)
	}
}
