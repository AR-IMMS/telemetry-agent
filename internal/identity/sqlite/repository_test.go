package sqlite_test

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
)

func TestRepositoryUsesSharedStateDatabaseAndKeepsFirstInstallationID(t *testing.T) {
	path := t.TempDir() + "/agent.db"
	store := agentstate.NewSQLiteStore(path)
	repository := identitysqlite.New(store.OpenDatabase)

	first, err := identity.ParseInstallationID("11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	second, err := identity.ParseInstallationID("21111111-2222-4333-8444-555555555555")
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

	compatibilityRepository := agentstate.NewIdentityStore(path)
	committed, err = compatibilityRepository.EnsureInstallationID(second)
	if err != nil {
		t.Fatalf("read ID through compatibility facade: %v", err)
	}
	if committed != first {
		t.Fatalf("compatibility facade returned %s, want %s", committed, first)
	}
}
