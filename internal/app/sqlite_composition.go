package app

import (
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app/migrations"
	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/identity/enrollment"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation"
	sqlitemigrate "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// NewAgentSQLiteStore constructs the Agent's shared SQLite adapter with
// context-owned migrations registered at the Application composition boundary.
func NewAgentSQLiteStore(path string) agentstate.SQLiteStore {
	return agentstate.NewSQLiteStore(path, initializeAgentSQLiteSchema)
}

// NewAgentStateStore constructs the Agent state repository with the composed
// SQLite migration registry while preserving explicit legacy JSON import.
func NewAgentStateStore(path string) agentstate.Repository {
	return agentstate.NewStore(path, initializeAgentSQLiteSchema)
}

// NewInstallationIdentityStore constructs the Identity SQLite adapter using
// the Application's ordered migration registry.
func NewInstallationIdentityStore(path string) installationid.InstallationIdentityRepository {
	return agentstate.NewIdentityStore(path, initializeAgentSQLiteSchema)
}

// NewEnrollmentAttemptStore constructs the enrollment adapter using the
// Application's ordered migration registry.
func NewEnrollmentAttemptStore(path string) enrollment.AttemptRepository {
	return agentstate.NewEnrollmentStore(path, initializeAgentSQLiteSchema)
}

// NewCoreGatewayRuntimeStore constructs the Core Gateway adapter using the
// Application's ordered migration registry.
func NewCoreGatewayRuntimeStore(path string) coreconfig.GatewayRuntimeRepository {
	return agentstate.NewCoreGatewayStore(path, initializeAgentSQLiteSchema)
}

func initializeAgentSQLiteSchema(db *sql.DB) error {
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
		return fmt.Errorf("compose Agent SQLite migrations: %w", err)
	}
	return sqlitemigrate.Apply(db, registry)
}
