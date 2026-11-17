package sqlite_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
	activationsqlite "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

func TestWriteCandidateJournalPreservesOutcomeHistory(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mapping := candidateMapping()
	mapping.Outcome, mapping.Stage, mapping.RecoveryResult = "failed", "validation", "not_started"
	writeCandidateMapping(t, db, 5, mapping)

	mapping.Outcome, mapping.Stage, mapping.RecoveryResult = "applied", "readiness", "not_needed"
	writeCandidateMapping(t, db, 5, mapping)

	var outcome, stage, historyJSON string
	if err := db.QueryRow(`SELECT outcome,stage,history_json FROM activation_journal WHERE generation=5`).Scan(&outcome, &stage, &historyJSON); err != nil {
		t.Fatal(err)
	}
	if outcome != "applied" || stage != "readiness" {
		t.Fatalf("terminal journal = (%q, %q), want applied/readiness", outcome, stage)
	}
	var history []map[string]string
	if err := json.Unmarshal([]byte(historyJSON), &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0]["outcome"] != "failed" || history[0]["stage"] != "validation" || history[0]["recovery_result"] != "not_started" {
		t.Fatalf("history = %#v, want the prior failed validation evidence", history)
	}
}

func TestFindCandidateTargetUsesImmutableArtifactIdentity(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mapping := candidateMapping()
	mapping.Outcome = "failed"
	writeCandidateMapping(t, db, 8, mapping)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	generation, outcome, found, err := activationsqlite.FindCandidateTarget(tx, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if !found || generation != 8 || outcome != "failed" {
		t.Fatalf("target = (%d, %q, %t), want (8, failed, true)", generation, outcome, found)
	}
	mapping.Platform = "other-platform"
	if _, _, found, err := activationsqlite.FindCandidateTarget(tx, mapping); err != nil || found {
		t.Fatalf("different platform target found=%t, err=%v; want no match", found, err)
	}
}

func TestReadCandidateOutcomeRequiresAppliedGenerationEvidence(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mapping := candidateMapping()
	mapping.Outcome, mapping.Stage, mapping.RecoveryResult = "applied", "readiness", "not_needed"
	writeCandidateMapping(t, db, 3, mapping)

	var queryer interface {
		QueryRow(string, ...any) *sql.Row
	} = db
	got, generation, found, err := activationsqlite.ReadCandidateOutcome(queryer, mapping.DesiredSetRevision, mapping.BaselineFingerprint, mapping.CandidateDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !found || generation != 3 || got != mapping {
		t.Fatalf("candidate outcome = (%#v, %d, %t), want mapping at generation 3", got, generation, found)
	}
	if err := activationsqlite.ValidateCandidateAppliedGeneration(got, generation, 2); err == nil {
		t.Fatal("ValidateCandidateAppliedGeneration() accepted stale applied evidence")
	}
	if err := activationsqlite.ValidateCandidateAppliedGeneration(got, generation, 3); err != nil {
		t.Fatalf("ValidateCandidateAppliedGeneration() error = %v, want nil", err)
	}
}

func writeCandidateMapping(t *testing.T, db *sql.DB, generation uint64, mapping activationdomain.CandidateMapping) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := activationsqlite.WriteCandidateJournal(tx, generation, mapping); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func candidateMapping() activationdomain.CandidateMapping {
	return activationdomain.CandidateMapping{
		DesiredSetRevision:    2,
		CandidateDigest:       "candidate-digest",
		BaselineFingerprint:   "baseline-fingerprint",
		EffectiveConfigDigest: "effective-digest",
		CollectorVersion:      "0.160.0",
		ManifestVersion:       "0.160.0",
		Platform:              "windows/amd64",
	}
}
