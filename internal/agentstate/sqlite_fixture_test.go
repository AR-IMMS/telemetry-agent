package agentstate

import (
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/app/migrations"
	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
	sqlitemigrate "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

func newTestSQLiteStore(path string) SQLiteStore {
	return NewSQLiteStoreWithGatewayApplied(path, initializeTestSQLiteSchema, remoteconfigsqlite.MarkGatewayAppliedInTransaction)
}

func newTestStore(path string) Repository {
	return NewStoreWithGatewayApplied(path, initializeTestSQLiteSchema, remoteconfigsqlite.MarkGatewayAppliedInTransaction)
}

func newTestIdentityStore(path string) installationid.InstallationIdentityRepository {
	return NewIdentityStore(path, initializeTestSQLiteSchema)
}

func newTestEnrollmentStore(path string) enrollment.AttemptRepository {
	return NewEnrollmentStore(path, initializeTestSQLiteSchema)
}

func newTestCoreGatewayStore(path string) coreconfig.GatewayRuntimeRepository {
	return NewCoreGatewayStore(path, initializeTestSQLiteSchema)
}

func initializeTestSQLiteSchema(db *sql.DB) error {
	registry, err := migrations.BuildAgentRegistry(migrations.Hooks{
		InitializeAgentState: InitializeBaseSQLiteSchema,
		ReadAgentState: func(tx *sql.Tx) (remoteconfigsqlite.AgentState, error) {
			state, err := LoadStateForMigration(tx)
			if err != nil {
				return remoteconfigsqlite.AgentState{}, err
			}
			return remoteconfigsqlite.AgentState{
				BootstrapEndpoint:   state.Collector.GatewayEndpoint,
				DesiredGeneration:   state.DesiredGeneration,
				ActivatedGeneration: state.ActivatedGeneration,
				AppliedGeneration:   state.AppliedGeneration,
			}, nil
		},
	})
	if err != nil {
		return fmt.Errorf("compose test SQLite migrations: %w", err)
	}
	return sqlitemigrate.Apply(db, registry)
}
