package sqlite_test

import (
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"testing"
)

func TestRepositoryUsesSharedStateDatabaseAndKeepsFirstInstallationID(t *testing.T) {
	path := t.TempDir() + "/agent.db"
	store := sqlitefixture.NewSQLiteStore(path)
	repository := identitysqlite.New(store.OpenDatabase)

	first, err := installationid.ParseInstallationID("11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	second, err := installationid.ParseInstallationID("21111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}

	committed, err := repository.EnsureInstallationID(first)
	if err != nil {
		t.Fatalf("initialize installation ID: %v", err)
	}
	if committed != first {
		t.Fatalf("first committed ID = %s, want %s", committed, first)
	}

	committed, err = repository.EnsureInstallationID(second)
	if err != nil {
		t.Fatalf("read committed installation ID: %v", err)
	}
	if committed != first {
		t.Fatalf("repeated initialization replaced ID with %s, want %s", committed, first)
	}

	compatibilityRepository := sqlitefixture.NewIdentityStore(path)
	committed, err = compatibilityRepository.EnsureInstallationID(second)
	if err != nil {
		t.Fatalf("read ID through compatibility facade: %v", err)
	}
	if committed != first {
		t.Fatalf("compatibility facade returned %s, want %s", committed, first)
	}
}
