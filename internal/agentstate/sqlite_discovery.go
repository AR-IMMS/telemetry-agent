package agentstate

import (
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverysqlite "github.com/ar-imms/telemetry-agent/internal/discovery/sqlite"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
	inventorysqlite "github.com/ar-imms/telemetry-agent/internal/inventory/sqlite"
)

// InventoryRepository returns the Inventory-owned SQLite adapter over this store.
func (s SQLiteStore) InventoryRepository() inventoryctx.InventoryRepository {
	return inventorysqlite.NewInventoryRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// ProposalRepository returns the transitional proposal SQLite adapter over this store.
func (s SQLiteStore) ProposalRepository() discovery.ProposalRepository {
	return discoverysqlite.NewProposalRepository(s.OpenDatabase, &sqliteUpdateMutex)
}
