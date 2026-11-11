package agentstate

import (
	"database/sql"
	"fmt"
)

func migrateIntegrationTemplates(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Integration Template migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=13`).Scan(&applied); err != nil {
		return fmt.Errorf("check Integration Template schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	statements := []string{
		`CREATE TABLE integration_templates (
			template_id TEXT PRIMARY KEY,
			payload BLOB NOT NULL CHECK(length(payload)<=65536),
			revision INTEGER NOT NULL CHECK(revision>0),
			status TEXT NOT NULL CHECK(status IN ('enabled','disabled','deleted')),
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE integration_template_state (
			id INTEGER PRIMARY KEY CHECK(id=1),
			template_set_revision INTEGER NOT NULL CHECK(template_set_revision>=0),
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO integration_template_state(id,template_set_revision,updated_at) VALUES(1,0,CURRENT_TIMESTAMP)`,
		`INSERT INTO schema_migrations(version,applied_at) VALUES(13,CURRENT_TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("create Integration Template schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Integration Template migration: %w", err)
	}
	return nil
}
