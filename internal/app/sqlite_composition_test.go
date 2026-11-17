package app

import (
	"errors"
	"path/filepath"
	"testing"

	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
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

func TestInventoryRepositoryUsesComposedSQLiteDatabase(t *testing.T) {
	repository := NewInventoryRepository(filepath.Join(t.TempDir(), "state.db"))
	if _, err := repository.Snapshot(); err != nil {
		t.Fatalf("read initial Inventory snapshot: %v", err)
	}
	if _, ok := repository.(inventoryapp.ProviderStatusReader); !ok {
		t.Fatal("composed Inventory adapter does not expose provider health evidence")
	}
}

func TestProposalRepositoryUsesComposedSQLiteDatabase(t *testing.T) {
	repository := NewProposalRepository(filepath.Join(t.TempDir(), "state.db"))
	proposals, err := repository.ProposalSnapshot()
	if err != nil {
		t.Fatalf("read initial Configuration proposals: %v", err)
	}
	if len(proposals) != 0 {
		t.Fatalf("initial Configuration proposals=%d, want 0", len(proposals))
	}
}

func TestIdentityRepositoriesUseComposedSQLiteDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	identity, err := installationid.ParseInstallationID("11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := NewInstallationIdentityStore(path).EnsureInstallationID(identity)
	if err != nil {
		t.Fatalf("initialize installation identity: %v", err)
	}
	if committed != identity {
		t.Fatalf("committed identity = %q, want %q", committed.String(), identity.String())
	}
	if _, err := NewEnrollmentAttemptStore(path).LoadAttempt(); !errors.Is(err, enrollment.ErrNoAttempt) {
		t.Fatalf("LoadAttempt() error = %v, want ErrNoAttempt", err)
	}
}
