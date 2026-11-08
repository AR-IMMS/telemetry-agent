package agentstate

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestCandidateAdmissionAndAppliedMappingAreAtomic(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error {
		state.DesiredGeneration = 4
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(0, 4, "baseline-a", "candidate-a")
	if _, err := store.AdmitCandidate(request); err != nil {
		t.Fatalf("AdmitCandidate() error = %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 5 || state.PendingActivation == nil || state.PendingActivation.Candidate == nil {
		t.Fatalf("admitted state = %#v, want generation 5 and candidate pending", state)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkGenerationActivated(5) }); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCandidateGenerationApplied(5, ""); err != nil {
		t.Fatalf("MarkCandidateGenerationApplied() error = %v", err)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != 5 || state.PendingActivation != nil {
		t.Fatalf("terminal state = %#v, want applied generation 5 and no pending activation", state)
	}
	var outcome, digest string
	if err := store.openAndQuery(`SELECT outcome,effective_config_digest FROM activation_journal WHERE generation=5`, &outcome, &digest); err != nil {
		t.Fatal(err)
	}
	if outcome != "applied" || digest != request.Mapping.EffectiveConfigDigest {
		t.Fatalf("journal outcome/digest = %q/%q", outcome, digest)
	}

	request.ExpectedGeneration = 5
	request.VerifiedFingerprint = "baseline-a"
	result, err := store.AdmitCandidate(request)
	if err != nil || !result.AlreadyApplied || result.Generation != 5 {
		t.Fatalf("idempotent AdmitCandidate() = %#v, %v", result, err)
	}
	state, _ = store.Load()
	if state.DesiredGeneration != 5 {
		t.Fatalf("idempotent admission advanced generation to %d", state.DesiredGeneration)
	}
}

func TestCandidateAdmissionRejectsStaleDesiredAndM2Revisions(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error { state.DesiredGeneration = 9; return nil }); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(1, 8, "baseline-a", "candidate-a")
	if _, err := store.AdmitCandidate(request); !errors.Is(err, ErrStaleCandidate) {
		t.Fatalf("stale admission error = %v, want ErrStaleCandidate", err)
	}
	request.DesiredSetRevision = 0
	request.Mapping.DesiredSetRevision = 0
	if _, err := store.AdmitCandidate(request); !errors.Is(err, ErrStaleCandidate) {
		t.Fatalf("stale M2 admission error = %v, want ErrStaleCandidate", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 9 || state.PendingActivation != nil {
		t.Fatalf("stale admission mutated state: %#v", state)
	}
}

func TestCandidateFailureReusesReservedGenerationOnRetry(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error { state.DesiredGeneration = 2; return nil }); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(0, 2, "baseline-a", "candidate-a")
	first, err := store.AdmitCandidate(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteCandidateFailure(first.Generation, "final_validation", "failed", "not_started"); err != nil {
		t.Fatal(err)
	}
	request.ExpectedGeneration = first.Generation
	second, err := store.AdmitCandidate(request)
	if err != nil {
		t.Fatalf("retry admission error = %v", err)
	}
	if second.Generation != first.Generation {
		t.Fatalf("retry generation = %d, want reused %d", second.Generation, first.Generation)
	}
	state, _ := store.Load()
	if state.DesiredGeneration != first.Generation || state.PendingActivation == nil {
		t.Fatalf("retry state = %#v", state)
	}
	if _, err := store.Update(func(state *State) error {
		return state.MarkCandidateGenerationActivated(second.Generation)
	}); err != nil {
		t.Fatalf("activate retried candidate: %v", err)
	}
	if err := store.MarkCandidateGenerationApplied(second.Generation, ""); err != nil {
		t.Fatalf("apply retried candidate: %v", err)
	}
	var history string
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT history_json FROM activation_journal WHERE generation=?`, second.Generation).Scan(&history); err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		Outcome        string `json:"outcome"`
		Stage          string `json:"stage"`
		RecoveryResult string `json:"recovery_result"`
	}
	if err := json.Unmarshal([]byte(history), &entries); err != nil {
		t.Fatalf("decode activation history %q: %v", history, err)
	}
	if len(entries) == 0 || entries[0].Outcome != "failed" || entries[0].Stage != "final_validation" || entries[0].RecoveryResult != "not_started" {
		t.Fatalf("activation history = %#v, want the original failed attempt preserved", entries)
	}
}

func TestCandidateGenerationCanCommitAfterNewerDesiredGenerationArrives(t *testing.T) {
	state := State{DesiredGeneration: 8, PendingActivation: &PendingActivation{
		Generation: 8,
		Candidate:  &CandidateMapping{CandidateDigest: "candidate"},
	}}
	state.DesiredGeneration = 9
	if err := state.MarkCandidateGenerationActivated(8); err != nil {
		t.Fatalf("MarkCandidateGenerationActivated() error = %v", err)
	}
	if state.ActivatedGeneration != 8 || state.DesiredGeneration != 9 {
		t.Fatalf("state = %#v, want admitted generation 8 activated with desired 9 outstanding", state)
	}
	if err := state.MarkGenerationActivated(8); err == nil {
		t.Fatal("legacy MarkGenerationActivated accepted a stale generation")
	}
}

func TestCandidateAppliedMappingRollsBackWithStateOnJournalFailure(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error { state.DesiredGeneration = 2; return nil }); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(0, 2, "baseline", "candidate")
	if _, err := store.AdmitCandidate(request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkCandidateGenerationActivated(3) }); err != nil {
		t.Fatalf("MarkCandidateGenerationActivated() error = %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 3 {
		t.Fatalf("reserved generation = %d, want 3", state.DesiredGeneration)
	}
	if err := store.openAndExec(`CREATE TRIGGER reject_candidate_terminal BEFORE UPDATE ON activation_journal WHEN NEW.outcome='applied' BEGIN SELECT RAISE(ABORT,'test journal failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkCandidateGenerationApplied(3, ""); err == nil {
		t.Fatal("MarkCandidateGenerationApplied() error = nil, want journal failure")
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != 0 || state.PendingActivation == nil {
		t.Fatalf("journal failure partially committed candidate state: %#v", state)
	}
}

func TestCandidateGatewayReadinessCommitsMappingWithAppliedGeneration(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error { state.DesiredGeneration = 5; return nil }); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(0, 5, "baseline-gateway", "candidate-gateway")
	admission, err := store.AdmitCandidate(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkCandidateGenerationActivated(admission.Generation) }); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkGatewayApplied(admission.Generation, "127.0.0.1:4317"); err != nil {
		t.Fatalf("MarkGatewayApplied() error = %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != admission.Generation || state.PendingActivation != nil {
		t.Fatalf("Gateway-applied state = %#v", state)
	}
	var outcome, digest string
	db, err := store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT outcome,effective_config_digest FROM activation_journal WHERE generation=?`, admission.Generation).Scan(&outcome, &digest); err != nil {
		t.Fatal(err)
	}
	if outcome != "applied" || digest != request.Mapping.EffectiveConfigDigest {
		t.Fatalf("Gateway candidate journal = %q/%q", outcome, digest)
	}
}

func TestCandidateAppliedGenerationRequiresAtomicTerminalMapping(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error { state.DesiredGeneration = 1; return nil }); err != nil {
		t.Fatal(err)
	}
	request := candidateAdmissionForTest(0, 1, "baseline", "candidate")
	admission, err := store.AdmitCandidate(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkCandidateGenerationActivated(admission.Generation) }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *State) error { return state.MarkGenerationApplied(admission.Generation) }); !errors.Is(err, ErrCandidateTerminalRequired) {
		t.Fatalf("generic AppliedGeneration update error = %v, want ErrCandidateTerminalRequired", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != 0 || state.PendingActivation == nil {
		t.Fatalf("generic update partially acknowledged candidate: %#v", state)
	}
}

func candidateAdmissionForTest(desiredRevision, generation uint64, baseline, candidate string) CandidateAdmission {
	return CandidateAdmission{
		DesiredSetRevision:  desiredRevision,
		ExpectedGeneration:  generation,
		ExpectedFingerprint: baseline,
		VerifiedFingerprint: baseline,
		RecoveryPath:        "recovery.yaml",
		Mapping: CandidateMapping{
			DesiredSetRevision:    desiredRevision,
			BaselineFingerprint:   baseline,
			CandidateDigest:       candidate,
			EffectiveConfigDigest: "effective-" + candidate,
			CollectorVersion:      "0.160.0",
			ManifestVersion:       "test",
			Platform:              "linux/amd64",
		},
	}
}

func (s SQLiteStore) openAndQuery(query string, destinations ...any) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	return db.QueryRow(query).Scan(destinations...)
}

func (s SQLiteStore) openAndExec(query string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(query)
	return err
}
