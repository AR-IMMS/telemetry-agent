package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// Migrations returns Runtime-owned Collector activation schema changes in their historical versions.
func Migrations() []migrate.Migration {
	return []migrate.Migration{
		{Version: 7, Name: "Candidate activation mapping", Apply: migrateCandidateActivationMapping},
		{Version: 8, Name: "Candidate attempt history", Apply: migrateCandidateAttemptHistory},
	}
}

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

func migrateCandidateAttemptHistory(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin candidate attempt history migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=8`).Scan(&applied); err != nil {
		return fmt.Errorf("check candidate attempt history schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	if _, err := tx.Exec(`ALTER TABLE activation_journal ADD COLUMN history_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
		return fmt.Errorf("add candidate attempt history to activation journal: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(8,CURRENT_TIMESTAMP)`); err != nil {
		return fmt.Errorf("record candidate attempt history schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit candidate attempt history migration: %w", err)
	}
	return nil
}
