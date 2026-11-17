package agentstate

import (
	"database/sql"

	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	reconciliationcontracts "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
)

// TemplateRepository returns the Configuration-owned Template adapter.
func (s SQLiteStore) TemplateRepository() templatesapp.Repository {
	return configurationsqlite.NewTemplateRepository(s.OpenDatabase, platformsqlite.UpdateLocker())
}

// MaterializationRepository returns the Configuration-owned projection adapter.
func (s SQLiteStore) MaterializationRepository() materializationapp.Repository {
	return configurationsqlite.NewMaterializationRepository(s.OpenDatabase, platformsqlite.UpdateLocker())
}

// MaterializationRepositoryWithEvents adds an optional post-commit advisory wakeup.
func (s SQLiteStore) MaterializationRepositoryWithEvents(publisher interface {
	Publish(events.DesiredConfigurationChanged) error
}) materializationapp.Repository {
	return configurationsqlite.NewMaterializationRepositoryWithEvents(s.OpenDatabase, platformsqlite.UpdateLocker(), publisher)
}

// ApprovalRepository returns the Configuration-owned atomic approval adapter.
func (s SQLiteStore) ApprovalRepository() application.ApprovalRepository {
	return configurationsqlite.NewApprovalRepository(s.OpenDatabase, platformsqlite.UpdateLocker())
}

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
