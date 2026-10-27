package agentstate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSQLiteStorePersistsStateAndUsesWAL(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error {
		state.DesiredGeneration = 3
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.DesiredGeneration != 3 {
		t.Fatalf("DesiredGeneration = %d, want 3", got.DesiredGeneration)
	}
	db, err := store.open()
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}
}

func TestSQLiteStoreImportsLegacyJSONState(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "state.json")
	legacy := NewFileStore(legacyPath)
	want := State{
		DesiredGeneration: 4,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: true,
				Ownership: &OwnershipRecord{Resources: []OwnedResource{{
					Kind: "systemd-service", Identifier: "node-exporter",
				}}},
			},
		},
	}
	if err := legacy.Save(want); err != nil {
		t.Fatalf("save legacy state: %v", err)
	}
	store := NewSQLiteStore(filepath.Join(directory, "state.db"))
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Dependencies["node-exporter"].Ownership.Resources[0].Identifier != "node-exporter" ||
		got.DesiredGeneration != want.DesiredGeneration {
		t.Fatalf("migrated state = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy state was not preserved: %v", err)
	}
}

func TestSQLiteStoreSerializesConcurrentUpdatesAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	first := NewSQLiteStore(path)
	second := NewSQLiteStore(path)
	const updates = 20
	var wait sync.WaitGroup
	errors := make(chan error, updates)
	for i := 0; i < updates; i++ {
		wait.Add(1)
		store := first
		if i%2 != 0 {
			store = second
		}
		go func() {
			defer wait.Done()
			_, err := store.Update(func(state *State) error {
				state.DesiredGeneration++
				return nil
			})
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent Update() error = %v", err)
		}
	}
	got, err := first.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.DesiredGeneration != updates {
		t.Fatalf("DesiredGeneration = %d, want %d", got.DesiredGeneration, updates)
	}
}

func TestSQLiteStoreJournalsActivationAndRollback(t *testing.T) {
	store := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if _, err := store.Update(func(state *State) error {
		state.DesiredGeneration = 2
		return state.MarkGenerationActivated(2)
	}); err != nil {
		t.Fatalf("activate generation: %v", err)
	}
	if _, err := store.Update(func(state *State) error {
		state.AppliedGeneration = 1
		return state.RollbackUnappliedActivation(2)
	}); err != nil {
		t.Fatalf("roll back activation: %v", err)
	}
	db, err := store.open()
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	defer db.Close()
	var outcome string
	if err := db.QueryRow(
		`SELECT outcome FROM activation_journal WHERE generation = 2`,
	).Scan(&outcome); err != nil {
		t.Fatalf("read activation journal: %v", err)
	}
	if outcome != "rolled_back" {
		t.Fatalf("activation outcome = %q, want rolled_back", outcome)
	}
}

func TestNewStoreMigratesExplicitLegacyJSONPathToSQLite(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "custom-state.json")
	if err := NewFileStore(legacyPath).Save(State{DesiredGeneration: 7}); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}
	store := NewStore(legacyPath)
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.DesiredGeneration != 7 {
		t.Fatalf("DesiredGeneration = %d, want 7", got.DesiredGeneration)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy JSON was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(legacyPath), "custom-state.db")); err != nil {
		t.Fatalf("SQLite database was not created: %v", err)
	}
}
