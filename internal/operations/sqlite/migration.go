package sqlite

import (
	"database/sql"
	"fmt"

	sqlitemigrate "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// Migrations returns the Operations-owned schema definition at its established version.
func Migrations() []sqlitemigrate.Migration {
	return []sqlitemigrate.Migration{{
		Version: 14,
		Name:    "Operations execution and outbox",
		Apply:   migrateExecutionSchema,
	}}
}

func migrateExecutionSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Operations execution migration: %w", err)
	}
	defer tx.Rollback()
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=14`).Scan(&applied); err != nil {
		return fmt.Errorf("check Operations execution schema version: %w", err)
	}
	if applied != 0 {
		return tx.Commit()
	}
	statements := []string{
		`CREATE TABLE operation_execution (
			operation_id TEXT PRIMARY KEY,
			operation_type TEXT NOT NULL,
			schema_version TEXT NOT NULL,
			request_hash TEXT NOT NULL,
			installation_id TEXT NOT NULL,
			binding_id TEXT NOT NULL,
			target_kind TEXT NOT NULL DEFAULT '',
			target_provider_id TEXT NOT NULL DEFAULT '',
			target_resource_id TEXT NOT NULL DEFAULT '',
			requested_by TEXT NOT NULL DEFAULT '',
			authorization_ref TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL CHECK(status IN ('RUNNING','SUCCEEDED','FAILED','UNKNOWN_OUTCOME')),
		received_at TEXT NOT NULL,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			expires_at TEXT NOT NULL,
			result_json TEXT CHECK(result_json IS NULL OR length(result_json)<=65536),
			error_code TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE operation_audit_event (
			event_id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL REFERENCES operation_execution(operation_id),
			sequence_number INTEGER NOT NULL CHECK(sequence_number>0),
			event_type TEXT NOT NULL,
			occurred_at TEXT NOT NULL,
			details_json TEXT NOT NULL CHECK(length(details_json)<=16384),
			UNIQUE(operation_id, sequence_number)
		)`,
		`CREATE TABLE operation_result_outbox (
			event_id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL REFERENCES operation_execution(operation_id),
			event_version TEXT NOT NULL,
			subject TEXT NOT NULL,
			payload_json TEXT NOT NULL CHECK(length(payload_json)<=65536),
			created_at TEXT NOT NULL,
			published_at TEXT,
			attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
			next_attempt_at TEXT NOT NULL,
			last_error_code TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX operation_result_outbox_pending ON operation_result_outbox(published_at,next_attempt_at,created_at)`,
		`INSERT INTO schema_migrations(version,applied_at) VALUES(14,CURRENT_TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("create Operations execution schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Operations execution migration: %w", err)
	}
	return nil
}
