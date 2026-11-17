package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app/migrations"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	proposalapp "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	enrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/sqlite"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	inventorysqlite "github.com/ar-imms/telemetry-agent/internal/inventory/sqlite"
	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
	sqlitemigrate "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// NewAgentSQLiteStore constructs the Agent's shared SQLite adapter with
// context-owned migrations registered at the Application composition boundary.
func NewAgentSQLiteStore(path string) agentstate.SQLiteStore {
	return agentstate.NewSQLiteStoreWithGatewayApplied(path, initializeAgentSQLiteSchema, remoteconfigsqlite.MarkGatewayAppliedInTransaction)
}

// NewAgentStateStore constructs the Agent state repository with the composed
// SQLite migration registry while preserving explicit legacy JSON import.
func NewAgentStateStore(path string) agentstate.Repository {
	return agentstate.NewStoreWithGatewayApplied(path, initializeAgentSQLiteSchema, remoteconfigsqlite.MarkGatewayAppliedInTransaction)
}

// NewInstallationIdentityStore constructs the Identity SQLite adapter using
// the Application's ordered migration registry.
func NewInstallationIdentityStore(path string) installationid.InstallationIdentityRepository {
	store := agentstate.NewSQLiteStoreForPath(path, initializeAgentSQLiteSchema)
	return identitysqlite.New(store.OpenDatabase)
}

// NewEnrollmentAttemptStore constructs the enrollment adapter using the
// Application's ordered migration registry.
func NewEnrollmentAttemptStore(path string) enrollment.AttemptRepository {
	store := agentstate.NewSQLiteStoreForPath(path, initializeAgentSQLiteSchema)
	return enrollmentsqlite.New(store.OpenDatabase, platformsqlite.UpdateLocker())
}

// NewInventoryRepository constructs the Inventory-owned SQLite adapter using
// the Application's ordered migration registry and shared transaction lock.
func NewInventoryRepository(path string) inventoryapp.InventoryRepository {
	store := agentstate.NewSQLiteStoreForPath(path, initializeAgentSQLiteSchema)
	return inventorysqlite.NewInventoryRepository(store.OpenDatabase, platformsqlite.UpdateLocker())
}

// NewProposalRepository constructs the Configuration-owned proposal adapter
// using the Application's ordered migration registry and shared SQLite lock.
func NewProposalRepository(path string) proposalapp.ProposalRepository {
	store := agentstate.NewSQLiteStoreForPath(path, initializeAgentSQLiteSchema)
	return configurationsqlite.NewProposalRepository(store.OpenDatabase, platformsqlite.UpdateLocker())
}

// NewDesiredStateRepository constructs the Configuration-owned desired-state
// adapter using the Application's ordered migration registry and shared lock.
func NewDesiredStateRepository(path string) desiredstateapp.Repository {
	store := agentstate.NewSQLiteStoreForPath(path, initializeAgentSQLiteSchema)
	return configurationsqlite.NewIntegrationRepository(store.OpenDatabase, platformsqlite.UpdateLocker())
}

// NewCoreGatewayRuntimeStore constructs the Core Gateway adapter using the
// Application's ordered migration registry.
func NewCoreGatewayRuntimeStore(path string) coreconfig.GatewayRuntimeRepository {
	store := NewAgentSQLiteStore(path)
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
				BootstrapEndpoint:   state.Collector.GatewayEndpoint,
				DesiredGeneration:   state.DesiredGeneration,
				ActivatedGeneration: state.ActivatedGeneration,
				AppliedGeneration:   state.AppliedGeneration,
			}, nil
		},
		SetDesiredGeneration:  agentstate.SetDesiredGenerationInTransaction,
		MarkGenerationApplied: agentstate.MarkGenerationAppliedInTransaction,
	})
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
