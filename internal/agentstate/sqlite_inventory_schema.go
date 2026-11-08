package agentstate

import (
	"database/sql"
	"fmt"
)

func migrateDiscoveryInventory(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Discovery inventory migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=10`).Scan(&applied); err != nil {
		return fmt.Errorf("check Discovery inventory schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	statements := []string{
		`CREATE TABLE inventory_meta (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL CHECK(revision>=0))`,
		`INSERT INTO inventory_meta(id,revision) VALUES(1,0)`,
		`CREATE TABLE inventory_resources (
			provider_id TEXT NOT NULL, provider_instance_id TEXT NOT NULL, provider_resource_id TEXT NOT NULL,
			payload BLOB NOT NULL, updated_at TEXT NOT NULL,
			PRIMARY KEY(provider_id,provider_instance_id,provider_resource_id))`,
		`CREATE TABLE discovery_provider_status (
			provider_id TEXT NOT NULL, provider_instance_id TEXT NOT NULL, payload BLOB NOT NULL, updated_at TEXT NOT NULL,
			PRIMARY KEY(provider_id,provider_instance_id))`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("create Discovery inventory schema: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(10,CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record Discovery inventory schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Discovery inventory migration: %w", err)
	}
	return nil
}
