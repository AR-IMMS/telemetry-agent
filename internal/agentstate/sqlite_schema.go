package agentstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type sqliteExecutor interface {
	Exec(string, ...any) (sql.Result, error)
}

// ensureSQLiteSchema applies idempotent schema setup before any state access.
func ensureSQLiteSchema(db sqliteExecutor) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS agent_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			payload BLOB NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS activation_journal (
			generation INTEGER PRIMARY KEY,
			outcome TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (1, CURRENT_TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("initialize Agent SQLite schema: %w", err)
		}
	}
	return nil
}

// importLegacyState imports JSON only when the database has no state row.
// The source file is retained so migration does not destroy the old recovery
// copy if the Agent is interrupted or the new database later proves unusable.
func (s SQLiteStore) importLegacyState(db *sql.DB) error {
	if s.legacyPath == "" || s.legacyPath == s.path {
		return nil
	}
	if _, err := loadSQLiteState(db); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content, err := os.ReadFile(s.legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy Agent state %q: %w", s.legacyPath, err)
	}
	var state State
	if err := json.Unmarshal(content, &state); err != nil {
		return fmt.Errorf("decode legacy Agent state %q: %w", s.legacyPath, err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode migrated Agent state: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Agent state migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO agent_state (id, payload, updated_at) VALUES (1, ?, ?) ON CONFLICT(id) DO NOTHING`,
		encoded,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("import legacy Agent state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy Agent state migration: %w", err)
	}
	return nil
}

// recordActivationTransition journals only changes to the activated revision.
// A lower revision represents restoring the previously activated generation.
func recordActivationTransition(
	tx *sql.Tx,
	previous State,
	current State,
) error {
	outcome := ""
	generation := current.ActivatedGeneration
	switch {
	case current.ActivatedGeneration > previous.ActivatedGeneration:
		outcome = "activated"
	case current.ActivatedGeneration < previous.ActivatedGeneration:
		outcome = "rolled_back"
		generation = previous.ActivatedGeneration
	}
	if outcome == "" {
		return nil
	}
	if _, err := tx.Exec(
		`INSERT INTO activation_journal (generation, outcome, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(generation) DO UPDATE SET outcome = excluded.outcome, updated_at = excluded.updated_at`,
		generation,
		outcome,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record configuration activation: %w", err)
	}
	return nil
}
