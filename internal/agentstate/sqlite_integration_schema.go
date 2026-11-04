package agentstate

import (
	"database/sql"
	"fmt"
)

func migrateIntegrationDesiredState(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Integration schema migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 6`).Scan(&applied); err != nil {
		return fmt.Errorf("check Integration schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	if _, err := tx.Exec(`CREATE TABLE integration_instances (instance_id TEXT PRIMARY KEY, payload BLOB NOT NULL, revision INTEGER NOT NULL CHECK (revision > 0), updated_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create Integration instances: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE integration_desired_state (id INTEGER PRIMARY KEY CHECK (id = 1), desired_set_revision INTEGER NOT NULL CHECK (desired_set_revision >= 0), updated_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create Integration desired state: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO integration_desired_state (id, desired_set_revision, updated_at) VALUES (1, 0, CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("initialize Integration desired state: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (6, CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record Integration schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Integration schema migration: %w", err)
	}
	return nil
}
