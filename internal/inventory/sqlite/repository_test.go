package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	inventorysqlite "github.com/ar-imms/telemetry-agent/internal/inventory/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestInventorySnapshotContextHonorsCancellationBeforeOpeningDatabase(t *testing.T) {
	opened := false
	repository := inventorysqlite.NewInventoryRepository(func() (*sql.DB, error) {
		opened = true
		return nil, errors.New("database should not be opened")
	}, &sync.Mutex{})
	reader, ok := repository.(interface {
		SnapshotContext(context.Context) (inventorydomain.InventorySnapshot, error)
	})
	if !ok {
		t.Fatal("Inventory SQLite repository does not expose cancellable snapshots")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.SnapshotContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("SnapshotContext() error = %v, want context.Canceled", err)
	}
	if opened {
		t.Fatal("canceled inventory snapshot opened its database")
	}
}

func TestInventoryRepositoryKeepsFailureEvidenceAndWriteCAS(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repository := inventorysqlite.NewInventoryRepository(store.OpenDatabase, &sync.Mutex{})
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	snapshot := inventorydomain.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []inventorydomain.ResourceObservation{{ProviderResourceID: "container-a", Kind: inventorydomain.ResourceContainer}},
	}
	resources := []inventorydomain.Resource{{
		Key:  inventorydomain.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Kind: inventorydomain.ResourceContainer, Presence: inventorydomain.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
	}}
	if err := repository.CommitProviderSnapshot(0, snapshot, resources, true); err != nil {
		t.Fatalf("commit complete provider snapshot: %v", err)
	}
	before, err := repository.Snapshot()
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if before.Revision != 1 || before.WriteRevision != 1 || len(before.Resources) != 1 {
		t.Fatalf("initial snapshot = %+v", before)
	}
	if err := repository.RecordProviderFailure(inventorydomain.ProviderStatus{
		ProviderID: "docker", ProviderInstanceID: "local", Enabled: true,
		LastAttemptAt: at.Add(time.Minute), LastErrorCode: "daemon_unavailable",
	}); err != nil {
		t.Fatalf("record provider failure: %v", err)
	}
	if err := repository.CommitProviderSnapshot(before.WriteRevision, snapshot, resources, false); !errors.Is(err, inventoryapp.ErrSnapshotConflict) {
		t.Fatalf("commit with stale write revision = %v, want snapshot conflict", err)
	}
	after, err := repository.Snapshot()
	if err != nil {
		t.Fatalf("read inventory after provider failure: %v", err)
	}
	if after.Revision != before.Revision || after.WriteRevision != before.WriteRevision+1 || len(after.Resources) != 1 ||
		after.Providers[0].LastErrorCode != "daemon_unavailable" || !after.Providers[0].LastSuccessAt.Equal(at) {
		t.Fatalf("failure changed material evidence or lost provider status: %+v", after)
	}
}

func TestInventoryRepositoryReadsBoundedProviderStatusesWithoutResources(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repository := inventorysqlite.NewInventoryRepository(store.OpenDatabase, &sync.Mutex{})
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	for index := 0; index < 3; index++ {
		instance := inventorydomain.ProviderInstanceID(fmt.Sprintf("instance-%d", index))
		resourceID := fmt.Sprintf("container-%d", index)
		providerSnapshot := inventorydomain.ProviderSnapshot{
			ProviderID: "docker", ProviderInstanceID: instance, Complete: true, ObservedAt: at,
			Resources: []inventorydomain.ResourceObservation{{ProviderResourceID: resourceID, Kind: inventorydomain.ResourceContainer}},
		}
		resource := inventorydomain.Resource{
			Key:  inventorydomain.ResourceKey{Provider: "docker", ProviderInstance: instance, ProviderResourceID: resourceID},
			Kind: inventorydomain.ResourceContainer, Presence: inventorydomain.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
		}
		if err := repository.CommitProviderSnapshot(uint64(index), providerSnapshot, []inventorydomain.Resource{resource}, true); err != nil {
			t.Fatalf("commit provider snapshot %d: %v", index, err)
		}
	}

	reader, ok := repository.(inventoryapp.ProviderStatusReader)
	if !ok {
		t.Fatal("inventory repository does not expose bounded provider status reader")
	}
	statuses, truncated, err := reader.ReadProviderStatuses(context.Background(), 2)
	if err != nil {
		t.Fatalf("read limited provider statuses: %v", err)
	}
	if len(statuses) != 2 || !truncated {
		t.Fatalf("limited statuses=(%d, truncated=%t), want (2, true)", len(statuses), truncated)
	}
	statuses, truncated, err = reader.ReadProviderStatuses(context.Background(), inventoryapp.MaxProviderStatusReadLimit)
	if err != nil {
		t.Fatalf("read provider statuses: %v", err)
	}
	if len(statuses) != 3 || truncated {
		t.Fatalf("all statuses=(%d, truncated=%t), want (3, false)", len(statuses), truncated)
	}
	for _, limit := range []int{0, inventoryapp.MaxProviderStatusReadLimit + 1} {
		if _, _, err := reader.ReadProviderStatuses(context.Background(), limit); err == nil {
			t.Fatalf("ReadProviderStatuses(%d) error=nil, want invalid limit error", limit)
		}
	}
}

func TestInventoryRepositoryProviderStatusReadHonorsCancellation(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	repository := inventorysqlite.NewInventoryRepository(store.OpenDatabase, &sync.Mutex{})
	reader := repository.(inventoryapp.ProviderStatusReader)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := reader.ReadProviderStatuses(ctx, inventoryapp.MaxProviderStatusReadLimit)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadProviderStatuses error=%v, want caller cancellation", err)
	}
}
