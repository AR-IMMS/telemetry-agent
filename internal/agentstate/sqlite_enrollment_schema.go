package agentstate

import (
	"database/sql"
	"fmt"
)

// migrateEnrollmentAttempts creates the secret-free singleton attempt record.
// Its table and schema version are committed atomically.
func migrateEnrollmentAttempts(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin enrollment schema migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 5`).Scan(&applied); err != nil {
		return fmt.Errorf("check enrollment schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='enrollment_attempt'`).Scan(&existing); err != nil {
		return fmt.Errorf("check enrollment attempt table: %w", err)
	}
	if existing != 0 {
		return fmt.Errorf("enrollment attempt table exists without schema migration version 5")
	}
	if _, err := tx.Exec(`CREATE TABLE enrollment_attempt (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		payload BLOB NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create enrollment attempt table: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (5, CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record enrollment schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit enrollment schema migration: %w", err)
	}
	return nil
}
