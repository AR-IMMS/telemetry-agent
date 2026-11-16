package agentstate

import (
	"context"
	"database/sql"
	"fmt"

	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
)

func NewCoreGatewayStore(path string, initializeSchema func(*sql.DB) error) coreconfig.GatewayRuntimeRepository {
	store := newSQLiteStoreForPath(path, initializeSchema)
	return newTestCoreGatewayRepository(store)
}

func newTestCoreGatewayRepository(store SQLiteStore) coreconfig.GatewayRuntimeRepository {
	return remoteconfigsqlite.New(func(ctx context.Context) (*sql.DB, error) {
		return store.OpenDatabaseContext(ctx)
	}, SQLiteUpdateLocker(), remoteconfigsqlite.StateAccess{
		BeginSnapshotAcceptance: func() (func(), error) {
			state, err := store.Load()
			if err != nil {
				return nil, fmt.Errorf("load Agent state before Core Gateway acceptance: %w", err)
			}
			if state.Collector.ConfigPath == "" {
				return nil, nil
			}
			lifecycleLock, err := AcquireLifecycleLock(state.Collector.ConfigPath + ".lifecycle")
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
			state, err := LoadStateForMigration(tx)
			if err != nil {
				return remoteconfigsqlite.AgentState{}, err
			}
			return remoteconfigsqlite.AgentState{
				BootstrapEndpoint: state.Collector.GatewayEndpoint,
				DesiredGeneration: state.DesiredGeneration, ActivatedGeneration: state.ActivatedGeneration,
				AppliedGeneration: state.AppliedGeneration,
			}, nil
		},
		SetDesiredGeneration:  SetDesiredGenerationInTransaction,
		MarkGenerationApplied: MarkGenerationAppliedInTransaction,
	})
}

func (s SQLiteStore) AcceptGatewaySnapshot(snapshot coreconfig.GatewaySnapshot) (coreconfig.Acceptance, error) {
	return newTestCoreGatewayRepository(s).AcceptGatewaySnapshot(context.Background(), snapshot)
}

func (s SQLiteStore) GatewayForGeneration(generation uint64) (coreconfig.RuntimeGateway, error) {
	return newTestCoreGatewayRepository(s).GatewayForGeneration(context.Background(), generation)
}

func (s SQLiteStore) MarkGatewayActivated(generation uint64) error {
	return newTestCoreGatewayRepository(s).MarkGatewayActivated(context.Background(), generation)
}

func (s SQLiteStore) MarkGatewayApplied(generation uint64, endpoint string) error {
	return newTestCoreGatewayRepository(s).MarkGatewayApplied(context.Background(), generation, endpoint)
}

func (s SQLiteStore) EffectiveGateway() (coreconfig.EffectiveGateway, error) {
	return newTestCoreGatewayRepository(s).EffectiveGateway(context.Background())
}
