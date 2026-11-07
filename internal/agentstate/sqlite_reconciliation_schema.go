package agentstate

import (
	"database/sql"
	"fmt"
)

func migrateReconciliationDiagnostics(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin reconciliation diagnostics migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=9`).Scan(&applied); err != nil {
		return err
	}
	if applied != 0 {
		return tx.Commit()
	}
	statements := []string{
		`CREATE TABLE reconciliation_targets(
			target_key TEXT PRIMARY KEY, desired_revision INTEGER NOT NULL, desired_generation INTEGER NOT NULL,
			baseline_fingerprint TEXT NOT NULL, retry_epoch INTEGER NOT NULL DEFAULT 0,
			attempt_count INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, stage TEXT, code TEXT, class TEXT,
			candidate_digest TEXT, lifecycle_generation INTEGER, safe_detail TEXT, retry_at TEXT, updated_at TEXT NOT NULL)`,
		`CREATE TABLE reconciliation_attempts(
			id INTEGER PRIMARY KEY AUTOINCREMENT, target_key TEXT NOT NULL, retry_epoch INTEGER NOT NULL,
			attempt_number INTEGER NOT NULL, status TEXT NOT NULL, started_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			stage TEXT, code TEXT, class TEXT, candidate_digest TEXT, lifecycle_generation INTEGER, safe_detail TEXT, retry_at TEXT,
			FOREIGN KEY(target_key) REFERENCES reconciliation_targets(target_key))`,
		`CREATE INDEX reconciliation_attempts_recent ON reconciliation_attempts(id DESC)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("create reconciliation diagnostics schema: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(9,CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reconciliation diagnostics migration: %w", err)
	}
	return nil
}
