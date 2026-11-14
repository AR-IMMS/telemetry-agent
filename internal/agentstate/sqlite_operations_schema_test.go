package agentstate

import (
	"path/filepath"
	"testing"
)

func TestOperationsMigrationAddsExecutionJournalAndOutbox(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 14 {
		t.Fatalf("schema version=%d err=%v, want 14", version, err)
	}
	for _, table := range []string{"operation_execution", "operation_audit_event", "operation_result_outbox"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table %q count=%d err=%v", table, count, err)
		}
	}
}
