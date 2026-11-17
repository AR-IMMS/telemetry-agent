package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// GatewayMigrationState is the legacy Agent state needed to seed Runtime's
// Gateway activation evidence while preserving the shared migration transaction.
type GatewayMigrationState struct {
	BootstrapEndpoint   string
	ActivatedGeneration uint64
	AppliedGeneration   uint64
}

// AcceptedGatewaySnapshot is the validated Configuration snapshot consumed by
// the Runtime migration. The composition boundary supplies its reader.
type AcceptedGatewaySnapshot struct {
	Revision   uint64
	Endpoint   string
	Generation uint64
}

// GatewayMigrationStateReader reads legacy Agent state within the migration transaction.
type GatewayMigrationStateReader func(*sql.Tx) (GatewayMigrationState, error)

// AcceptedGatewaySnapshotReader reads Configuration-owned evidence without transferring table ownership.
type AcceptedGatewaySnapshotReader func(*sql.Tx) (AcceptedGatewaySnapshot, bool, error)

// CoreGatewayRuntimeMigration creates Runtime-owned Gateway activation evidence
// at its historical schema version. Context readers are composed by the app layer.
func CoreGatewayRuntimeMigration(readState GatewayMigrationStateReader, readSnapshot AcceptedGatewaySnapshotReader) migrate.Migration {
	return migrate.Migration{
		Version: 4,
		Name:    "Core Gateway runtime",
		Apply: func(db *sql.DB) error {
			return migrateCoreGatewayRuntime(db, readState, readSnapshot)
		},
	}
}

func migrateCoreGatewayRuntime(db *sql.DB, readState GatewayMigrationStateReader, readSnapshot AcceptedGatewaySnapshotReader) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Core Gateway runtime migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=4`).Scan(&applied); err != nil {
		return fmt.Errorf("check Core Gateway runtime schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='core_gateway_runtime'`).Scan(&existing); err != nil {
		return fmt.Errorf("check Core Gateway runtime table: %w", err)
	}
	if existing != 0 {
		return errors.New("Core Gateway runtime table exists without schema migration version 4")
	}
	if _, err := tx.Exec(`CREATE TABLE core_gateway_runtime (
		id INTEGER PRIMARY KEY CHECK (id=1), last_good_endpoint TEXT NOT NULL,
		activated_revision INTEGER NOT NULL CHECK (activated_revision>=0),
		activated_generation INTEGER NOT NULL CHECK (activated_generation>=0),
		applied_revision INTEGER NOT NULL CHECK (applied_revision>=0),
		applied_generation INTEGER NOT NULL CHECK (applied_generation>=0))`); err != nil {
		return fmt.Errorf("create Core Gateway runtime table: %w", err)
	}
	if readState != nil {
		state, stateErr := readState(tx)
		if stateErr == nil {
			if readSnapshot == nil {
				return errors.New("Configuration Gateway snapshot reader is required when legacy Runtime state exists")
			}
			gateway, exists, readErr := readSnapshot(tx)
			if readErr != nil {
				return fmt.Errorf("read accepted Gateway snapshot during Runtime migration: %w", readErr)
			}
			if exists {
				lastGood, activatedRevision, appliedRevision := state.BootstrapEndpoint, uint64(0), uint64(0)
				if state.ActivatedGeneration >= gateway.Generation {
					activatedRevision = gateway.Revision
				}
				if state.AppliedGeneration >= gateway.Generation {
					lastGood, appliedRevision = gateway.Endpoint, gateway.Revision
				}
				if _, err := tx.Exec(`INSERT INTO core_gateway_runtime(id,last_good_endpoint,activated_revision,activated_generation,applied_revision,applied_generation) VALUES(1,?,?,?,?,?)`, lastGood, activatedRevision, state.ActivatedGeneration, appliedRevision, state.AppliedGeneration); err != nil {
					return fmt.Errorf("seed Core Gateway runtime from existing state: %w", err)
				}
			}
		} else if !errors.Is(stateErr, os.ErrNotExist) {
			return fmt.Errorf("load Agent state during Core Gateway runtime migration: %w", stateErr)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(4,CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record Core Gateway runtime schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Core Gateway runtime migration: %w", err)
	}
	return nil
}
