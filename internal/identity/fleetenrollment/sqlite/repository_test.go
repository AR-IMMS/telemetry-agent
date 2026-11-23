package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
	_ "modernc.org/sqlite"
)

func TestRepositoryPersistsSecretFreeFleetAttemptAndCredentialReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	open := func() (*sql.DB, error) {
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_txlock=immediate")
		if err == nil {
			db.SetMaxOpenConns(1)
		}
		return db, err
	}
	initialize, err := open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := initialize.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := Migrations()[0].Apply(initialize); err != nil {
		t.Fatal(err)
	}
	_ = initialize.Close()
	repository := New(open, &sync.Mutex{})
	attempt := domain.Attempt{AttemptID: "11111111-1111-4111-8111-111111111111", AgentID: "22222222-2222-4222-8222-222222222222", Origin: "https://fleet.example.test", InvitationRef: "invite-ref", RecoveryProofRef: "proof-ref", CredentialRef: "credential-ref", Status: domain.AttemptPending}
	if err := repository.CreateAttempt(attempt); err != nil {
		t.Fatalf("CreateAttempt(): %v", err)
	}
	loaded, err := repository.LoadAttempt()
	if err != nil || loaded != attempt {
		t.Fatalf("LoadAttempt()=%+v, %v", loaded, err)
	}
	result := domain.Result{AgentID: attempt.AgentID, NodeID: "33333333-3333-4333-8333-333333333333", CredentialID: "cred", CredentialScope: "opamp.connect", OpAMPEndpoint: "wss://fleet.example.test/opamp"}
	if err := repository.CompleteAttempt(attempt.AttemptID, result); err != nil {
		t.Fatalf("CompleteAttempt(): %v", err)
	}
	loaded, err = repository.LoadAttempt()
	if err != nil || loaded.Status != domain.AttemptRegistered || loaded.Result == nil || loaded.Result.CredentialID != result.CredentialID {
		t.Fatalf("completed attempt=%+v err=%v", loaded, err)
	}
	if reference, err := repository.CurrentCredentialReference(); err != nil || reference != attempt.CredentialRef {
		t.Fatalf("CurrentCredentialReference()=%q, %v", reference, err)
	}
	databaseBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(databaseBytes)
	if string(databaseBytes) == "" {
		t.Fatal("SQLite database is empty")
	}
}

func TestRepositoryRejectsSecondPendingAttemptAndChangedCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.db")
	open := func() (*sql.DB, error) {
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_txlock=immediate")
		if err == nil {
			db.SetMaxOpenConns(1)
		}
		return db, err
	}
	initialize, _ := open()
	_, _ = initialize.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`)
	if err := Migrations()[0].Apply(initialize); err != nil {
		t.Fatal(err)
	}
	_ = initialize.Close()
	repository := New(open, &sync.Mutex{})
	attempt := domain.Attempt{AttemptID: "11111111-1111-4111-8111-111111111111", AgentID: "22222222-2222-4222-8222-222222222222", Origin: "https://fleet.example.test", InvitationRef: "i", RecoveryProofRef: "p", CredentialRef: "c", Status: domain.AttemptPending}
	if err := repository.CreateAttempt(attempt); err != nil {
		t.Fatal(err)
	}
	changed := attempt
	changed.AttemptID = "44444444-4444-4444-8444-444444444444"
	if err := repository.CreateAttempt(changed); err == nil {
		t.Fatal("second pending attempt accepted")
	}
}
