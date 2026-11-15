package sqlite

import "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"

// Migrations returns Configuration schema changes in their historical order.
func Migrations() []migrate.Migration {
	return []migrate.Migration{
		{Version: 6, Name: "Integration desired state", Apply: migrateIntegrationDesiredState},
		{Version: 9, Name: "Reconciliation diagnostics", Apply: migrateReconciliationDiagnostics},
		{Version: 12, Name: "Monitoring proposals", Apply: migrateMonitoringProposals},
		{Version: 13, Name: "Integration templates", Apply: migrateIntegrationTemplates},
	}
}
