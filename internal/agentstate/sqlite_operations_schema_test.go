package agentstate

import (
	"path/filepath"
	"testing"
)

func TestOperationsMigrationAddsExecutionJournalAndOutbox(t *testing.T) {
	store := newTestSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 14 {
		t.Fatalf("schema version=%d err=%v, want 14", version, err)
	}
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	for want := 1; want <= 14; want++ {
		if !rows.Next() {
			rows.Close()
			t.Fatalf("migration registry ended before version %d", want)
		}
		var got int
		if err := rows.Scan(&got); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if got != want {
			rows.Close()
			t.Fatalf("migration registry version=%d, want %d", got, want)
		}
	}
	if rows.Next() {
		var extra int
		if err := rows.Scan(&extra); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
		t.Fatalf("unexpected migration version %d after 14", extra)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"operation_execution", "operation_audit_event", "operation_result_outbox"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table %q count=%d err=%v", table, count, err)
		}
	}
}
