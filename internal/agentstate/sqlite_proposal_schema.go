package agentstate

import (
	"database/sql"
	"fmt"
)

func migrateMonitoringProposals(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin monitoring proposal migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=12`).Scan(&applied); err != nil {
		return fmt.Errorf("check monitoring proposal schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	if _, err := tx.Exec(`CREATE TABLE integration_proposals (
		proposal_id TEXT PRIMARY KEY,
		provider_id TEXT NOT NULL,
		provider_instance_id TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		integration_id TEXT NOT NULL,
		revision INTEGER NOT NULL CHECK(revision>0),
		status TEXT NOT NULL CHECK(status IN ('pending','needs_input','dismissed','approved','stale')),
		payload BLOB NOT NULL CHECK(length(payload)<=65536),
		updated_at TEXT NOT NULL,
		UNIQUE(provider_id,provider_instance_id,resource_id,integration_id)
	)`); err != nil {
		return fmt.Errorf("create monitoring proposal schema: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(12,CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record monitoring proposal migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit monitoring proposal migration: %w", err)
	}
	return nil
}
