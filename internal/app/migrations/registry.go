package migrations

import (
	"database/sql"
	"fmt"

	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	identityenrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/sqlite"
	fleetenrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/sqlite"
	identityinstallationsqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	inventorysqlite "github.com/ar-imms/telemetry-agent/internal/inventory/sqlite"
	operationssqlite "github.com/ar-imms/telemetry-agent/internal/operations/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
	activationsqlite "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/sqlite"
)

// Hooks supplies Agent-owned schema/state semantics used when composing
// context migrations. The callbacks do not transfer table ownership.
type Hooks struct {
	InitializeAgentState        func(*sql.DB) error
	ReadAgentState              func(*sql.Tx) (remoteconfigsqlite.AgentState, error)
	ReadAcceptedGatewaySnapshot func(*sql.Tx) (activationsqlite.AcceptedGatewaySnapshot, bool, error)
}

// BuildAgentRegistry returns the historical contiguous schema registry in
// version order, using migration functions owned by their respective contexts.
func BuildAgentRegistry(hooks Hooks) ([]migrate.Migration, error) {
	if hooks.InitializeAgentState == nil {
		return nil, fmt.Errorf("Agent base schema initializer is required")
	}
	if hooks.ReadAgentState == nil {
		return nil, fmt.Errorf("Agent migration state reader is required")
	}

	configurationMigrations := configurationsqlite.Migrations()
	migration6, err := migrationForVersion(configurationMigrations, 6)
	if err != nil {
		return nil, err
	}
	migration9, err := migrationForVersion(configurationMigrations, 9)
	if err != nil {
		return nil, err
	}
	migration12, err := migrationForVersion(configurationMigrations, 12)
	if err != nil {
		return nil, err
	}
	migration13, err := migrationForVersion(configurationMigrations, 13)
	if err != nil {
		return nil, err
	}

	registry := []migrate.Migration{{
		Version: 1,
		Name:    "Agent state and activation journal",
		Apply: func(db *sql.DB) error {
			if err := migrate.InitializeLedger(db); err != nil {
				return err
			}
			if err := hooks.InitializeAgentState(db); err != nil {
				return fmt.Errorf("initialize Agent State base schema: %w", err)
			}
			if err := activationsqlite.InitializeBaseJournalSchema(db); err != nil {
				return err
			}
			return migrate.RecordApplied(db, 1)
		},
	}}
	registry = append(registry, identityinstallationsqlite.Migrations()...)
	registry = append(registry, remoteconfigsqlite.Migrations()...)
	registry = append(registry, activationsqlite.CoreGatewayRuntimeMigration(
		func(tx *sql.Tx) (activationsqlite.GatewayMigrationState, error) {
			state, err := hooks.ReadAgentState(tx)
			if err != nil {
				return activationsqlite.GatewayMigrationState{}, err
			}
			return activationsqlite.GatewayMigrationState{
				BootstrapEndpoint:   state.BootstrapEndpoint,
				ActivatedGeneration: state.ActivatedGeneration,
				AppliedGeneration:   state.AppliedGeneration,
			}, nil
		},
		hooks.ReadAcceptedGatewaySnapshot,
	))
	registry = append(registry, identityenrollmentsqlite.Migrations()...)
	registry = append(registry, migration6)
	registry = append(registry, activationsqlite.Migrations()...)
	registry = append(registry, migration9)
	registry = append(registry, inventorysqlite.Migrations()...)
	registry = append(registry, migration12, migration13)
	registry = append(registry, operationssqlite.Migrations()...)
	registry = append(registry, fleetenrollmentsqlite.Migrations()...)
	return registry, nil
}

func migrationForVersion(registry []migrate.Migration, version int) (migrate.Migration, error) {
	for _, migration := range registry {
		if migration.Version == version {
			return migration, nil
		}
	}
	return migrate.Migration{}, fmt.Errorf("Configuration SQLite migration %d is not registered", version)
}
