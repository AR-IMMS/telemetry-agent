package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenCreatesDirectoryAndAppliesSharedSQLiteDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.db")
	initialized := false
	db, err := Open(path, func(db *sql.DB) error {
		initialized = true
		_, err := db.Exec(`CREATE TABLE compatibility_probe (value TEXT NOT NULL)`)
		return err
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if !initialized {
		t.Fatal("schema initializer was not called")
	}
	for pragma, want := range map[string]string{"journal_mode": "wal", "synchronous": "2"} {
		var got string
		if err := db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", pragma, err)
		}
		if got != want {
			t.Errorf("PRAGMA %s = %q, want %q", pragma, got, want)
		}
	}
	var busyTimeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
	if _, err := db.Exec(`INSERT INTO compatibility_probe(value) VALUES ('preserved')`); err != nil {
		t.Fatalf("use initialized database: %v", err)
	}
}

func TestOpenClosesDatabaseWhenInitializationFails(t *testing.T) {
	want := errors.New("fixture migration failure")
	if db, err := Open(filepath.Join(t.TempDir(), "state.db"), func(*sql.DB) error { return want }); !errors.Is(err, want) {
		if db != nil {
			db.Close()
		}
		t.Fatalf("Open() error = %v, want %v", err, want)
	}
}
