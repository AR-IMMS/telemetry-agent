package sqlitefixture

import (
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app/migrations"
	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/identity/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity/installation"
	sqlitemigrate "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// NewSQLiteStore returns a fixture store using the production migration registry.
func NewSQLiteStore(path string) agentstate.SQLiteStore {
	return agentstate.NewSQLiteStore(path, initialize)
}

// NewStore returns a fixture Agent state repository.
func NewStore(path string) agentstate.Repository {
	return agentstate.NewStore(path, initialize)
}

// NewIdentityStore returns a fixture Installation Identity repository.
func NewIdentityStore(path string) installation.InstallationIdentityRepository {
	return agentstate.NewIdentityStore(path, initialize)
}

// NewEnrollmentStore returns a fixture Enrollment repository.
func NewEnrollmentStore(path string) enrollment.AttemptRepository {
	return agentstate.NewEnrollmentStore(path, initialize)
}

// NewCoreGatewayStore returns a fixture Core Gateway runtime repository.
func NewCoreGatewayStore(path string) coreconfig.GatewayRuntimeRepository {
	return agentstate.NewCoreGatewayStore(path, initialize)
}

func initialize(db *sql.DB) error {
	registry, err := migrations.BuildAgentRegistry(migrations.Hooks{
		InitializeAgentState: agentstate.InitializeBaseSQLiteSchema,
		ReadAgentState: func(tx *sql.Tx) (remoteconfigsqlite.AgentState, error) {
			state, err := agentstate.LoadStateForMigration(tx)
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
