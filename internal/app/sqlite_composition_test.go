package app

import (
	"errors"
	"path/filepath"
	"testing"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	materializationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	proposalapp "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
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

func TestDesiredStateRepositoryUsesComposedSQLiteDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	repository := NewDesiredStateRepository(path)
	snapshot, err := repository.Snapshot()
	if err != nil {
		t.Fatalf("read initial Configuration desired state: %v", err)
	}
	if snapshot.Revision != 0 || len(snapshot.Instances) != 0 {
		t.Fatalf("initial desired state=%+v, want empty revision 0", snapshot)
	}
	if _, revision, err := repository.Put(desiredstatedomain.Instance{
		ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Enabled: true,
	}, 0); err != nil || revision != 1 {
		t.Fatalf("persist desired instance revision=%d err=%v", revision, err)
	}
	reopened, err := NewDesiredStateRepository(path).Snapshot()
	if err != nil || reopened.Revision != 1 || len(reopened.Instances) != 1 || reopened.Instances[0].ID != "cache" {
		t.Fatalf("reopened desired state=%+v err=%v", reopened, err)
	}
}

func TestTemplateRepositoryUsesComposedSQLiteDatabase(t *testing.T) {
	repository := NewTemplateRepository(filepath.Join(t.TempDir(), "state.db"))
	snapshot, err := repository.Snapshot()
	if err != nil {
		t.Fatalf("read initial Configuration Templates: %v", err)
	}
	if snapshot.Revision != 0 || len(snapshot.Templates) != 0 {
		t.Fatalf("initial Templates=%+v, want empty revision 0", snapshot)
	}
}

func TestMaterializationRepositoriesUseConfigurationAdapter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	var published int
	publisher := materializationPublisherFunc(func(events.DesiredConfigurationChanged) error {
		published++
		return nil
	})
	for name, repository := range map[string]materializationapp.Repository{
		"without event publisher": NewMaterializationRepository(path),
		"with event publisher":    NewMaterializationRepositoryWithEvents(path, publisher),
	} {
		if _, err := repository.ApplyTemplateProjection(materializationdomain.Batch{}); !errors.Is(err, materializationdomain.ErrInvalidMaterialization) {
			t.Errorf("%s invalid batch error=%v, want ErrInvalidMaterialization", name, err)
		}
	}
	if published != 0 {
		t.Fatalf("publisher called %d times for invalid batches", published)
	}
}

func TestApprovalRepositoryUsesConfigurationAdapter(t *testing.T) {
	repository := NewApprovalRepository(filepath.Join(t.TempDir(), "state.db"))
	if _, _, err := repository.CommitApproval(proposalapp.ApprovalCommit{}); !errors.Is(err, proposalapp.ErrInvalidProposal) {
		t.Fatalf("invalid approval commit error=%v, want ErrInvalidProposal", err)
	}
}

func TestReconciliationRepositoryUsesComposedSQLiteDatabase(t *testing.T) {
	repository := NewReconciliationRepository(filepath.Join(t.TempDir(), "state.db"))
	if _, err := repository.BeginAttempt(contracts.Target{
		DesiredRevision: 1, DesiredGeneration: 1, BaselineFingerprint: "baseline",
	}); err != nil {
		t.Fatalf("persist reconciliation attempt through composed repository: %v", err)
	}
	history, err := repository.History(10)
	if err != nil {
		t.Fatalf("read reconciliation history: %v", err)
	}
	if len(history) != 1 || history[0].Number != 1 || history[0].Status != "started" {
		t.Fatalf("reconciliation history=%+v, want one started attempt", history)
	}
}

type materializationPublisherFunc func(events.DesiredConfigurationChanged) error

func (f materializationPublisherFunc) Publish(event events.DesiredConfigurationChanged) error {
	return f(event)
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
