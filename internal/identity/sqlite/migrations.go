package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// Migrations returns Identity-owned schema changes in their historical versions.
func Migrations() []migrate.Migration {
	return []migrate.Migration{{Version: 2, Name: "Installation identity", Apply: migrateInstallationIdentity}}
}

// migrateInstallationIdentity atomically creates the identity schema and its
// explicit uninitialized singleton row, independently of UUID initialization.
func migrateInstallationIdentity(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Agent identity schema migration: %w", err)
	}
	defer tx.Rollback()

	var applied int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE version = 2`,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check Agent identity schema version: %w", err)
	}
	if applied != 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("finish Agent identity schema check: %w", err)
		}
		return nil
	}

	var tableExists int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'installation_identity'`,
	).Scan(&tableExists); err != nil {
		return fmt.Errorf("check Agent identity table: %w", err)
	}
	if tableExists != 0 {
		return fmt.Errorf("Agent identity table exists without schema migration version 2")
	}
	if _, err := tx.Exec(
		`CREATE TABLE installation_identity (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			installation_id TEXT,
			initialized INTEGER NOT NULL CHECK (initialized IN (0, 1)),
			CHECK (
				(initialized = 0 AND installation_id IS NULL) OR
				(initialized = 1 AND installation_id IS NOT NULL)
			)
		)`,
	); err != nil {
		return fmt.Errorf("create Agent identity table: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO installation_identity (id, installation_id, initialized)
		 VALUES (1, NULL, 0)`,
	); err != nil {
		return fmt.Errorf("initialize Agent identity row: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, applied_at) VALUES (2, CURRENT_TIMESTAMP)`,
	); err != nil {
		return fmt.Errorf("record Agent identity schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Agent identity schema migration: %w", err)
	}
	return nil
}
