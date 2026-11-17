package sqlitefixture

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app/migrations"
	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	enrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/sqlite"
	installation "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
	sqlitemigrate "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// NewSQLiteStore returns a fixture store using the production migration registry.
func NewSQLiteStore(path string) agentstate.SQLiteStore {
	return agentstate.NewSQLiteStoreWithGatewayApplied(path, initialize, remoteconfigsqlite.MarkGatewayAppliedInTransaction)
}

// NewStore returns a fixture Agent state repository.
func NewStore(path string) agentstate.Repository {
	return agentstate.NewStoreWithGatewayApplied(path, initialize, remoteconfigsqlite.MarkGatewayAppliedInTransaction)
}

// NewIdentityStore returns a fixture Installation Identity repository.
func NewIdentityStore(path string) installation.InstallationIdentityRepository {
	store := agentstate.NewSQLiteStoreForPath(path, initialize)
	return identitysqlite.New(store.OpenDatabase)
}

// NewEnrollmentStore returns a fixture Enrollment repository.
func NewEnrollmentStore(path string) enrollment.AttemptRepository {
	store := agentstate.NewSQLiteStoreForPath(path, initialize)
	return enrollmentsqlite.New(store.OpenDatabase, platformsqlite.UpdateLocker())
}

// NewCoreGatewayStore returns a fixture Core Gateway runtime repository.
func NewCoreGatewayStore(path string) coreconfig.GatewayRuntimeRepository {
	store := NewSQLiteStore(path)
	return remoteconfigsqlite.New(func(ctx context.Context) (*sql.DB, error) {
		return store.OpenDatabaseContext(ctx)
	}, platformsqlite.UpdateLocker(), remoteconfigsqlite.StateAccess{
		BeginSnapshotAcceptance: func() (func(), error) {
			state, err := store.Load()
			if err != nil {
				return nil, fmt.Errorf("load Agent state before Core Gateway acceptance: %w", err)
			}
			if state.Collector.ConfigPath == "" {
				return nil, nil
			}
			lifecycleLock, err := agentstate.AcquireLifecycleLock(state.Collector.ConfigPath + ".lifecycle")
			if err != nil {
				return nil, fmt.Errorf("acquire lifecycle lock for Core Gateway acceptance: %w", err)
			}
			return func() { _ = lifecycleLock.Close() }, nil
		},
		ReadInstallationID: func(tx *sql.Tx) (string, bool, error) {
			id, initialized, err := identitysqlite.ReadInstallationID(tx)
			return id.String(), initialized, err
		},
		Read: func(tx *sql.Tx) (remoteconfigsqlite.AgentState, error) {
			state, err := agentstate.LoadStateForMigration(tx)
			if err != nil {
				return remoteconfigsqlite.AgentState{}, err
			}
			return remoteconfigsqlite.AgentState{
				BootstrapEndpoint: state.Collector.GatewayEndpoint,
				DesiredGeneration: state.DesiredGeneration, ActivatedGeneration: state.ActivatedGeneration,
				AppliedGeneration: state.AppliedGeneration,
			}, nil
		},
		SetDesiredGeneration:  agentstate.SetDesiredGenerationInTransaction,
		MarkGenerationApplied: agentstate.MarkGenerationAppliedInTransaction,
	})
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
