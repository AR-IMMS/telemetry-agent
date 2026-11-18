package migrations

import (
	"database/sql"
	"errors"
	"testing"

	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	_ "modernc.org/sqlite"
)

func TestBuildAgentRegistryPreservesContiguousContextOwnedVersions(t *testing.T) {
	registry, err := BuildAgentRegistry(Hooks{
		InitializeAgentState: func(*sql.DB) error { return nil },
		ReadAgentState: func(*sql.Tx) (remoteconfigsqlite.AgentState, error) {
			return remoteconfigsqlite.AgentState{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		version int
		name    string
	}{
		{1, "Agent state and activation journal"},
		{2, "Installation identity"},
		{3, "Core Gateway snapshot"},
		{4, "Core Gateway runtime"},
		{5, "Enrollment attempts"},
		{6, "Integration desired state"},
		{7, "Candidate activation mapping"},
		{8, "Candidate attempt history"},
		{9, "Reconciliation diagnostics"},
		{10, "Discovery inventory"},
		{11, "Inventory write revision"},
		{12, "Monitoring proposals"},
		{13, "Integration templates"},
		{14, "Operations execution and outbox"},
	}
	if len(registry) != len(want) {
		t.Fatalf("migration registry length=%d, want %d", len(registry), len(want))
	}
	for index, expected := range want {
		if got := registry[index]; got.Version != expected.version || got.Name != expected.name || got.Apply == nil {
			t.Errorf("migration[%d]=(%d,%q,apply=%t), want (%d,%q,apply=true)", index, got.Version, got.Name, got.Apply != nil, expected.version, expected.name)
		}
	}
}

func TestBuildAgentRegistryRequiresOwnerCallbacks(t *testing.T) {
	_, err := BuildAgentRegistry(Hooks{})
	if err == nil {
		t.Fatal("BuildAgentRegistry() accepted missing context-owned callbacks")
	}
}

func TestVersionOneComposesTechnicalAgentAndRuntimeTablesBeforeRecordingVersion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	registry, err := BuildAgentRegistry(Hooks{
		InitializeAgentState: func(db *sql.DB) error {
			var ledgerCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&ledgerCount); err != nil {
				return err
			}
			if ledgerCount != 1 {
				return errors.New("migration ledger must be initialized before Agent State schema")
			}
			var versionOneRows int
			if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=1`).Scan(&versionOneRows); err != nil {
				return err
			}
			if versionOneRows != 0 {
				return errors.New("version 1 must not be recorded before Runtime schema")
			}
			if _, err := db.Exec(`CREATE TABLE agent_state (id INTEGER PRIMARY KEY CHECK (id = 1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
				return err
			}
			_, err := db.Exec(`CREATE TRIGGER require_runtime_journal_before_version_one
				BEFORE INSERT ON schema_migrations WHEN NEW.version=1
				BEGIN
					SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='activation_journal')
					THEN RAISE(ABORT, 'Runtime journal must exist before version 1 is recorded') END;
				END`)
			return err
		},
		ReadAgentState: func(*sql.Tx) (remoteconfigsqlite.AgentState, error) {
			return remoteconfigsqlite.AgentState{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry[0].Apply(db); err != nil {
		t.Fatalf("apply version 1: %v", err)
	}
	for _, table := range []string{"schema_migrations", "agent_state", "activation_journal"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("table %q count=%d, want 1", table, count)
		}
	}
	var versionRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=1`).Scan(&versionRows); err != nil {
		t.Fatal(err)
	}
	if versionRows != 1 {
		t.Fatalf("schema version 1 rows=%d, want 1", versionRows)
	}
}

func TestVersionOneDoesNotRecordVersionWhenAgentSchemaFails(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	wantErr := errors.New("agent schema unavailable")
	registry, err := BuildAgentRegistry(Hooks{
		InitializeAgentState: func(*sql.DB) error { return wantErr },
		ReadAgentState: func(*sql.Tx) (remoteconfigsqlite.AgentState, error) {
			return remoteconfigsqlite.AgentState{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry[0].Apply(db); !errors.Is(err, wantErr) {
		t.Fatalf("apply version 1 error=%v, want %v", err, wantErr)
	}
	var ledgerExists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&ledgerExists); err != nil {
		t.Fatal(err)
	}
	if ledgerExists != 1 {
		t.Fatalf("migration ledger exists=%d, want initialized technical ledger", ledgerExists)
	}
	var journalExists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='activation_journal'`).Scan(&journalExists); err != nil {
		t.Fatal(err)
	}
	if journalExists != 0 {
		t.Fatalf("activation journal exists=%d after Agent schema failure, want 0", journalExists)
	}
}
