package agentstate

import (
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	inventorysqlite "github.com/ar-imms/telemetry-agent/internal/inventory/sqlite"
)

// InventoryRepository returns the Inventory-owned SQLite adapter over this store.
func (s SQLiteStore) InventoryRepository() inventoryapp.InventoryRepository {
	return inventorysqlite.NewInventoryRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// ProposalRepository returns the Configuration-owned proposal adapter over this store.
func (s SQLiteStore) ProposalRepository() application.ProposalRepository {
	return configurationsqlite.NewProposalRepository(s.OpenDatabase, &sqliteUpdateMutex)
}
