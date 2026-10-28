package agentstate

import (
	"database/sql"
	"fmt"
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
