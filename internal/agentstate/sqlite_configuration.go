package agentstate

import (
	"database/sql"

	"github.com/ar-imms/telemetry-agent/internal/configuration"
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
func (s SQLiteStore) MaterializationRepository() configuration.MaterializationRepository {
	return configurationsqlite.NewMaterializationRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

// MaterializationRepositoryWithEvents adds an optional post-commit advisory wakeup.
func (s SQLiteStore) MaterializationRepositoryWithEvents(publisher interface {
	Publish(events.DesiredConfigurationChanged) error
}) configuration.MaterializationRepository {
	return configurationsqlite.NewMaterializationRepositoryWithEvents(s.OpenDatabase, &sqliteUpdateMutex, publisher)
}

// ApprovalRepository returns the Configuration-owned atomic approval adapter.
func (s SQLiteStore) ApprovalRepository() configuration.ApprovalRepository {
	return configurationsqlite.NewApprovalRepository(s.OpenDatabase, &sqliteUpdateMutex)
}

func desiredIntegrationRevision(queryer interface {
	QueryRow(string, ...any) *sql.Row
}) (uint64, error) {
	var revision uint64
	err := queryer.QueryRow(`SELECT desired_set_revision FROM integration_desired_state WHERE id=1`).Scan(&revision)
	return revision, err
}
