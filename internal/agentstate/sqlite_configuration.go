package agentstate

import (
	"database/sql"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	reconciliationcontracts "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
)

// IntegrationRepository returns the Configuration-owned desired-state adapter.
func (s SQLiteStore) IntegrationRepository() configuration.IntegrationRepository {
	return configurationsqlite.NewIntegrationRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// TemplateRepository returns the Configuration-owned Template adapter.
func (s SQLiteStore) TemplateRepository() configuration.TemplateRepository {
	return configurationsqlite.NewTemplateRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// MaterializationRepository returns the Configuration-owned projection adapter.
func (s SQLiteStore) MaterializationRepository() materializationapp.Repository {
	return configurationsqlite.NewMaterializationRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// MaterializationRepositoryWithEvents adds an optional post-commit advisory wakeup.
func (s SQLiteStore) MaterializationRepositoryWithEvents(publisher interface {
	Publish(events.DesiredConfigurationChanged) error
}) materializationapp.Repository {
	return configurationsqlite.NewMaterializationRepositoryWithEvents(s.OpenDatabase, &sqliteUpdateMutex, publisher)
}

// ApprovalRepository returns the Configuration-owned atomic approval adapter.
func (s SQLiteStore) ApprovalRepository() application.ApprovalRepository {
	return configurationsqlite.NewApprovalRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// ReconciliationRepository returns Configuration-owned reconciliation persistence.
func (s SQLiteStore) ReconciliationRepository() reconciliationcontracts.Repository {
	return configurationsqlite.NewReconciliationRepository(s.OpenDatabase, &sqliteUpdateMutex, s)
}

func desiredIntegrationRevision(queryer interface {
	QueryRow(string, ...any) *sql.Row
}) (uint64, error) {
	var revision uint64
	err := queryer.QueryRow(`SELECT desired_set_revision FROM integration_desired_state WHERE id=1`).Scan(&revision)
	return revision, err
}
