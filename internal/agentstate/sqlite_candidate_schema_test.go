package agentstate

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestCandidateJournalMigrationPreservesVersionSixLifecycleState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`,
		`CREATE TABLE agent_state(id INTEGER PRIMARY KEY CHECK(id=1),payload BLOB NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE activation_journal(generation INTEGER PRIMARY KEY,outcome TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO schema_migrations(version,applied_at) VALUES(1,CURRENT_TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, migrate := range []func(*sql.DB) error{
		migrateInstallationIdentity,
		migrateCoreGatewaySnapshot,
		migrateCoreGatewayRuntime,
		migrateEnrollmentAttempts,
		migrateIntegrationDesiredState,
	} {
		if err := migrate(db); err != nil {
			t.Fatalf("build version 6 fixture: %v", err)
		}
	}
	legacyState := State{DesiredGeneration: 8, ActivatedGeneration: 7, AppliedGeneration: 7, Dependencies: map[string]DependencyState{"node-exporter": {Enabled: true}}}
	encoded, err := json.Marshal(legacyState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_state(id,payload,updated_at) VALUES(1,?,?)`, encoded, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO activation_journal(generation,outcome,updated_at) VALUES(7,'activated',?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store := NewSQLiteStore(path)
	if _, err := store.Load(); err != nil {
		t.Fatalf("open version 6 database through version 8 migration: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 8 || state.ActivatedGeneration != 7 || state.AppliedGeneration != 7 || !state.Dependencies["node-exporter"].Enabled {
		t.Fatalf("migrated lifecycle state = %#v", state)
	}
	db, err = store.open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var outcome string
	var desiredRevision, candidateDigest, baselineFingerprint sql.NullString
	var history string
	if err := db.QueryRow(`SELECT outcome,CAST(desired_set_revision AS TEXT),candidate_digest,baseline_fingerprint,history_json FROM activation_journal WHERE generation=7`).Scan(&outcome, &desiredRevision, &candidateDigest, &baselineFingerprint, &history); err != nil {
		t.Fatal(err)
	}
	if outcome != "activated" || desiredRevision.Valid || candidateDigest.Valid || baselineFingerprint.Valid || history != "[]" {
		t.Fatalf("legacy journal after migration = %q / %#v / %#v / %#v / %q", outcome, desiredRevision, candidateDigest, baselineFingerprint, history)
	}
}
