package agentstate

import (
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverysqlite "github.com/ar-imms/telemetry-agent/internal/discovery/sqlite"
)

// InventoryRepository returns the Discovery-owned SQLite adapter over this store.
func (s SQLiteStore) InventoryRepository() discovery.InventoryRepository {
	return discoverysqlite.NewInventoryRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// ProposalRepository returns the Discovery-owned SQLite adapter over this store.
func (s SQLiteStore) ProposalRepository() discovery.ProposalRepository {
	return discoverysqlite.NewProposalRepository(s.OpenDatabase, &sqliteUpdateMutex)
}
