package agentstate

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
)

// migrateCoreGatewaySnapshot creates the ownership marker without seeding it.
// The table and migration version commit together; an existing unversioned
// table is rejected so a partial or foreign schema cannot become authority.
func migrateCoreGatewaySnapshot(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Core Gateway schema migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 3`).Scan(&applied); err != nil {
		return fmt.Errorf("check Core Gateway schema version: %w", err)
	}
	if applied != 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("finish Core Gateway schema check: %w", err)
		}
		return nil
	}
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'core_gateway_snapshot'`).Scan(&existing); err != nil {
		return fmt.Errorf("check Core Gateway snapshot table: %w", err)
	}
	if existing != 0 {
		return fmt.Errorf("Core Gateway snapshot table exists without schema migration version 3")
	}
	if _, err := tx.Exec(`CREATE TABLE core_gateway_snapshot (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		revision INTEGER NOT NULL CHECK (revision > 0),
		endpoint TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		accepted_at TEXT NOT NULL,
		target_generation INTEGER NOT NULL CHECK (target_generation >= 0)
	)`); err != nil {
		return fmt.Errorf("create Core Gateway snapshot table: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (3, CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record Core Gateway schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Core Gateway schema migration: %w", err)
	}
	return nil
}

// migrateCoreGatewayRuntime adds durable last-good endpoint metadata without
// changing or seeding the Bootstrap ownership marker.
func migrateCoreGatewayRuntime(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Core Gateway runtime migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 4`).Scan(&applied); err != nil {
		return fmt.Errorf("check Core Gateway runtime schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'core_gateway_runtime'`).Scan(&existing); err != nil {
		return fmt.Errorf("check Core Gateway runtime table: %w", err)
	}
	if existing != 0 {
		return fmt.Errorf("Core Gateway runtime table exists without schema migration version 4")
	}
	if _, err := tx.Exec(`CREATE TABLE core_gateway_runtime (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		last_good_endpoint TEXT NOT NULL,
		activated_revision INTEGER NOT NULL CHECK (activated_revision >= 0),
		activated_generation INTEGER NOT NULL CHECK (activated_generation >= 0),
		applied_revision INTEGER NOT NULL CHECK (applied_revision >= 0),
		applied_generation INTEGER NOT NULL CHECK (applied_generation >= 0)
	)`); err != nil {
		return fmt.Errorf("create Core Gateway runtime table: %w", err)
	}
	state, err := loadSQLiteState(tx)
	if err == nil {
		gateway, exists, loadErr := loadCoreGatewayRow(tx)
		if loadErr != nil {
			return loadErr
		}
		if exists {
			lastGood := state.Collector.GatewayEndpoint
			var activatedRevision uint64
			var appliedRevision uint64
			if state.ActivatedGeneration >= gateway.generation {
				activatedRevision = gateway.revision
			}
			if state.AppliedGeneration >= gateway.generation {
				lastGood = gateway.endpoint
				appliedRevision = gateway.revision
			}
			if _, err := tx.Exec(`INSERT INTO core_gateway_runtime
				(id, last_good_endpoint, activated_revision, activated_generation, applied_revision, applied_generation)
				VALUES (1, ?, ?, ?, ?, ?)`,
				lastGood, activatedRevision, state.ActivatedGeneration, appliedRevision, state.AppliedGeneration); err != nil {
				return fmt.Errorf("seed Core Gateway runtime from existing state: %w", err)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load Agent state during Core Gateway runtime migration: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (4, CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record Core Gateway runtime schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Core Gateway runtime migration: %w", err)
	}
	return nil
}
