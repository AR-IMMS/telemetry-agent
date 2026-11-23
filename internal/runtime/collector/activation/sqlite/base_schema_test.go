package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
	activationsqlite "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/sqlite"
)

func TestInitializeBaseJournalSchemaCreatesOnlyRuntimeJournalTable(t *testing.T) {
	db, err := platformsqlite.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"), func(*sql.DB) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for range 2 {
		if err := activationsqlite.InitializeBaseJournalSchema(db); err != nil {
			t.Fatalf("InitializeBaseJournalSchema() error = %v", err)
		}
	}
	var journalCount, migrationLedgerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='activation_journal'`).Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&migrationLedgerCount); err != nil {
		t.Fatal(err)
	}
	if journalCount != 1 || migrationLedgerCount != 0 {
		t.Fatalf("base tables = (journal=%d, migration ledger=%d), want (1, 0)", journalCount, migrationLedgerCount)
	}
}
