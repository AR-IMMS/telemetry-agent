package agentstate

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestSQLiteIdentityRepositoryInitializesAndReusesCommittedID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	wantState := State{
		Collector: CollectorContext{
			BinaryPath:      "otelcol",
			ConfigPath:      "otel.yaml",
			GatewayEndpoint: "127.0.0.1:4317",
			HealthEndpoint:  "http://127.0.0.1:13133",
		},
		DesiredGeneration:   4,
		ActivatedGeneration: 4,
		AppliedGeneration:   3,
		Dependencies: map[string]DependencyState{
			"node-exporter": {
				Enabled: true,
				Ownership: &OwnershipRecord{Resources: []OwnedResource{{
					Kind:       "systemd-service",
					Identifier: "node-exporter",
				}}},
			},
		},
	}
	if err := store.Save(wantState); err != nil {
		t.Fatalf("initialize M1 state: %v", err)
	}

	first := mustInstallationID(t, "11111111-2222-4333-8444-555555555555")
	second := mustInstallationID(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	repository := NewIdentityStore(path)
	got, err := repository.EnsureInstallationID(first)
	if err != nil {
		t.Fatalf("EnsureInstallationID(first) error = %v", err)
	}
	if got != first {
		t.Fatalf("first committed ID = %q, want %q", got, first)
	}
	got, err = repository.EnsureInstallationID(second)
	if err != nil {
		t.Fatalf("EnsureInstallationID(second) error = %v", err)
	}
	if got != first {
		t.Fatalf("reused ID = %q, want committed ID %q", got, first)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load M1 state: %v", err)
	}
	if !reflect.DeepEqual(state, wantState) {
		t.Fatalf("M1 state changed during identity initialization: %#v, want %#v", state, wantState)
	}
}

func TestSQLiteIdentityMigrationSeparatesSchemaAndInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store := NewSQLiteStore(path)
	if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() error = %v, want missing state", err)
	}

	db, err := store.open()
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer db.Close()
	var schemaVersion int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&schemaVersion); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if schemaVersion != 5 {
		t.Fatalf("schema version = %d, want 5", schemaVersion)
	}
	var coreSnapshots int
	if err := db.QueryRow("SELECT COUNT(*) FROM core_gateway_snapshot").Scan(&coreSnapshots); err != nil {
		t.Fatalf("read Core Gateway ownership row: %v", err)
	}
	if coreSnapshots != 0 {
		t.Fatalf("migration created %d Core Gateway ownership rows, want none", coreSnapshots)
	}
	var initialized int
	var installationID sql.NullString
	if err := db.QueryRow(
		"SELECT initialized, installation_id FROM installation_identity WHERE id = 1",
	).Scan(&initialized, &installationID); err != nil {
		t.Fatalf("read uninitialized identity row: %v", err)
	}
	if initialized != 0 || installationID.Valid {
		t.Fatalf("identity state = initialized:%d id:%v, want uninitialized row", initialized, installationID)
	}
}

func TestSQLiteIdentityConcurrentInitializationReturnsCommittedID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	first := NewIdentityStore(path)
	second := NewIdentityStore(path)
	candidates := []identity.InstallationID{
		mustInstallationID(t, "11111111-2222-4333-8444-555555555555"),
		mustInstallationID(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"),
	}
	results := make(chan identity.InstallationID, len(candidates))
	errorsFound := make(chan error, len(candidates))
	var wait sync.WaitGroup
	for index, repository := range []identity.InstallationIdentityRepository{first, second} {
		wait.Add(1)
		go func(index int, repository identity.InstallationIdentityRepository) {
			defer wait.Done()
			id, err := repository.EnsureInstallationID(candidates[index])
			if err != nil {
				errorsFound <- err
				return
			}
			results <- id
		}(index, repository)
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent EnsureInstallationID() error = %v", err)
	}
	var committed identity.InstallationID
	for id := range results {
		if committed.String() == "" {
			committed = id
			continue
		}
		if id != committed {
			t.Fatalf("concurrent IDs differ: %q and %q", committed, id)
		}
	}
	if committed != candidates[0] && committed != candidates[1] {
		t.Fatalf("committed ID = %q, want one initialization candidate", committed)
	}
}

func TestSQLiteIdentityDoesNotRecreateMissingInitializedRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	repository := NewIdentityStore(path)
	first := mustInstallationID(t, "11111111-2222-4333-8444-555555555555")
	if _, err := repository.EnsureInstallationID(first); err != nil {
		t.Fatalf("initialize identity: %v", err)
	}
	db, err := NewSQLiteStore(path).open()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := db.Exec("DELETE FROM installation_identity WHERE id = 1"); err != nil {
		db.Close()
		t.Fatalf("delete initialized row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	second := mustInstallationID(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	if _, err := repository.EnsureInstallationID(second); err == nil {
		t.Fatal("EnsureInstallationID() error = nil, want missing initialized row error")
	}
	verifyDB, err := NewSQLiteStore(path).open()
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer verifyDB.Close()
	var rows int
	if err := verifyDB.QueryRow("SELECT COUNT(*) FROM installation_identity").Scan(&rows); err != nil {
		t.Fatalf("count identity rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("identity rows = %d, want missing row to remain missing", rows)
	}
}

func TestSQLiteIdentityRejectsMalformedInitializedID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	repository := NewIdentityStore(path)
	first := mustInstallationID(t, "11111111-2222-4333-8444-555555555555")
	if _, err := repository.EnsureInstallationID(first); err != nil {
		t.Fatalf("initialize identity: %v", err)
	}
	db, err := NewSQLiteStore(path).open()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE installation_identity SET installation_id = 'malformed' WHERE id = 1",
	); err != nil {
		db.Close()
		t.Fatalf("corrupt identity row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if _, err := repository.EnsureInstallationID(
		mustInstallationID(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"),
	); err == nil {
		t.Fatal("EnsureInstallationID() error = nil, want malformed initialized ID error")
	}
}

func TestSQLiteIdentityStoreResolvesLegacyJSONPath(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "state.json")
	wantState := State{DesiredGeneration: 9}
	if err := NewFileStore(legacyPath).Save(wantState); err != nil {
		t.Fatalf("save legacy state: %v", err)
	}
	repository := NewIdentityStore(legacyPath)
	if _, err := repository.EnsureInstallationID(
		mustInstallationID(t, "11111111-2222-4333-8444-555555555555"),
	); err != nil {
		t.Fatalf("initialize identity from legacy state path: %v", err)
	}
	state, err := NewStore(legacyPath).Load()
	if err != nil {
		t.Fatalf("load imported state: %v", err)
	}
	if !reflect.DeepEqual(state, wantState) {
		t.Fatalf("state = %#v, want %#v", state, wantState)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy state backup missing: %v", err)
	}
}

func mustInstallationID(t *testing.T, value string) identity.InstallationID {
	t.Helper()
	id, err := identity.ParseInstallationID(value)
	if err != nil {
		t.Fatalf("ParseInstallationID(%q) error = %v", value, err)
	}
	return id
}
