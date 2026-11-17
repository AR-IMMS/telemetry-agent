package agentstate

import (
	"database/sql"

	reconciliationcontracts "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
)

// ReconciliationRepository returns Configuration-owned reconciliation persistence.
func (s SQLiteStore) ReconciliationRepository() reconciliationcontracts.Repository {
	return configurationsqlite.NewReconciliationRepository(s.OpenDatabase, platformsqlite.UpdateLocker(), s)
}

func desiredIntegrationRevision(queryer interface {
	QueryRow(string, ...any) *sql.Row
}) (uint64, error) {
	var revision uint64
	err := queryer.QueryRow(`SELECT desired_set_revision FROM integration_desired_state WHERE id=1`).Scan(&revision)
	return revision, err
}
