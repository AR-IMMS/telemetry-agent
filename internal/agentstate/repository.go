package agentstate

import (
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

// Repository loads and atomically updates Agent-owned state. Update serializes
// read-modify-write operations; callers must keep mutations limited to state
// changes and must not perform external I/O inside them.
type Repository interface {
	// Load returns the last committed state or an error when no state exists.
	Load() (State, error)
	// Update atomically applies a state-only mutation and returns the result.
	Update(StateMutation) (State, error)
}

// NewStore creates SQLite operational storage. An explicit legacy .json path is
// imported into a sibling .db file and preserved as migration backup.
func NewStore(path string) Repository {
	return newSQLiteStoreForPath(path)
}

// NewIdentityStore returns the SQLite adapter for installation identity at
// the same operational database path used by NewStore.
func NewIdentityStore(path string) identity.InstallationIdentityRepository {
	return newSQLiteStoreForPath(path)
}

// NewCoreGatewayStore returns the SQLite adapter for Core Gateway snapshot
// acceptance at the same operational database path used by NewStore.
func NewCoreGatewayStore(path string) coreconfig.GatewayRuntimeRepository {
	return newSQLiteStoreForPath(path)
}

func newSQLiteStoreForPath(path string) SQLiteStore {
	path = strings.TrimSpace(path)
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return SQLiteStore{
			path:       strings.TrimSuffix(path, filepath.Ext(path)) + ".db",
			legacyPath: path,
		}
	}
	return NewSQLiteStore(path)
}
