package migrate

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplyRunsRegisteredMigrationsInVersionOrder(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var applied []int
	migrations := []Migration{
		{Version: 1, Name: "base", Apply: func(*sql.DB) error { applied = append(applied, 1); return nil }},
		{Version: 2, Name: "identity", Apply: func(*sql.DB) error { applied = append(applied, 2); return nil }},
		{Version: 3, Name: "configuration", Apply: func(*sql.DB) error { applied = append(applied, 3); return nil }},
	}
	if err := Apply(db, migrations); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(applied, want) {
		t.Fatalf("applied versions = %v, want %v", applied, want)
	}
}

func TestApplyRejectsInvalidOrderBeforeRunningMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var called bool
	err = Apply(db, []Migration{
		{Version: 1, Name: "base", Apply: func(*sql.DB) error { called = true; return nil }},
		{Version: 3, Name: "skipped", Apply: func(*sql.DB) error { called = true; return nil }},
	})
	if err == nil {
		t.Fatal("Apply() accepted a migration list with a missing version")
	}
	if called {
		t.Fatal("Apply() ran a migration before validating the full list")
	}
}

func TestApplyStopsAndIdentifiesTheFailingMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	want := errors.New("migration fixture failed")
	var applied []int
	err = Apply(db, []Migration{
		{Version: 1, Name: "base", Apply: func(*sql.DB) error { applied = append(applied, 1); return nil }},
		{Version: 2, Name: "identity", Apply: func(*sql.DB) error { applied = append(applied, 2); return want }},
		{Version: 3, Name: "configuration", Apply: func(*sql.DB) error { applied = append(applied, 3); return nil }},
	})
	if !errors.Is(err, want) {
		t.Fatalf("Apply() error = %v, want wrapped %v", err, want)
	}
	if !reflect.DeepEqual(applied, []int{1, 2}) {
		t.Fatalf("applied versions = %v, want [1 2]", applied)
	}
	if err == nil || !strings.Contains(err.Error(), "version 2") || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("Apply() error = %v, want version and name context", err)
	}
}

func TestInitializeLedgerAndRecordAppliedAreIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := InitializeLedger(db); err != nil {
		t.Fatal(err)
	}
	if err := InitializeLedger(db); err != nil {
		t.Fatalf("InitializeLedger() second call error = %v", err)
	}
	if err := RecordApplied(db, 1); err != nil {
		t.Fatal(err)
	}
	if err := RecordApplied(db, 1); err != nil {
		t.Fatalf("RecordApplied() second call error = %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("version 1 rows = %d, want 1", count)
	}
}
