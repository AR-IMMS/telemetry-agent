package agentstate

import (
	"database/sql"
	"fmt"
)

func migrateCandidateActivationMapping(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin candidate activation schema migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=7`).Scan(&applied); err != nil {
		return fmt.Errorf("check candidate activation schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	columns := []string{
		`ALTER TABLE activation_journal ADD COLUMN desired_set_revision INTEGER`,
		`ALTER TABLE activation_journal ADD COLUMN candidate_digest TEXT`,
		`ALTER TABLE activation_journal ADD COLUMN baseline_fingerprint TEXT`,
		`ALTER TABLE activation_journal ADD COLUMN effective_config_digest TEXT`,
		`ALTER TABLE activation_journal ADD COLUMN collector_version TEXT`,
		`ALTER TABLE activation_journal ADD COLUMN manifest_version TEXT`,
		`ALTER TABLE activation_journal ADD COLUMN platform TEXT`,
		`ALTER TABLE activation_journal ADD COLUMN stage TEXT`,
		`ALTER TABLE activation_journal ADD COLUMN recovery_result TEXT`,
	}
	for _, statement := range columns {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("extend activation journal for candidate mapping: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(7,CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record candidate activation schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit candidate activation schema migration: %w", err)
	}
	return nil
}
