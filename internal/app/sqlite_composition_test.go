package app

import (
	"path/filepath"
	"testing"
)

func TestAgentSQLiteStoreRunsComposedMigrationsInOneDatabase(t *testing.T) {
	store := NewAgentSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 14 {
		t.Fatalf("latest schema version=%d, want 14", version)
	}
}
