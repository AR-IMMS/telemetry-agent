package agentstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// InitializeAgentStateBaseSchema creates the legacy Agent State payload table.
// Application migration composition creates the technical ledger, Runtime's
// activation journal, and records historical version 1 around this callback.
func InitializeAgentStateBaseSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS agent_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			payload BLOB NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("initialize Agent SQLite schema: %w", err)
		}
	}
	return nil
}

// LoadStateForMigration reads Agent-owned state from the migration transaction
// without committing or changing the transaction boundary.
func LoadStateForMigration(tx *sql.Tx) (State, error) {
	if tx == nil {
		return State{}, fmt.Errorf("Agent migration transaction is required")
	}
	return loadSQLiteState(tx)
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
