package application

import (
	"context"
	"errors"

	"github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
)

// ErrSnapshotConflict indicates another writer changed inventory before commit.
var ErrSnapshotConflict = errors.New("inventory snapshot changed concurrently")

// InventoryRepository persists inventory snapshots independently of desired state.
type InventoryRepository interface {
	Snapshot() (domain.InventorySnapshot, error)
	CommitProviderSnapshot(expectedWriteRevision uint64, snapshot domain.ProviderSnapshot, resources []domain.Resource, materialChange bool) error
	RecordProviderFailure(status domain.ProviderStatus) error
}

// MaxProviderStatusReadLimit caps provider-status-only diagnostic reads.
const MaxProviderStatusReadLimit = 64

// ProviderStatusReader reads a bounded provider-status projection without materializing resources.
type ProviderStatusReader interface {
	// ReadProviderStatuses returns no more than limit rows and reports whether additional rows were omitted.
	ReadProviderStatuses(ctx context.Context, limit int) ([]domain.ProviderStatus, bool, error)
}

// DiscoveryProvider returns one complete snapshot or an explicit error.
type DiscoveryProvider interface {
	ID() domain.ProviderID
	InstanceID() domain.ProviderInstanceID
	Discover(ctx context.Context) (domain.ProviderSnapshot, error)
}
