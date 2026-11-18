package sqlite

import (
	"database/sql"
	"fmt"
)

// InitializeBaseJournalSchema creates the Runtime-owned base activation journal
// table. It does not create the migration ledger or own migration ordering.
func InitializeBaseJournalSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("SQLite database is required for activation journal schema")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS activation_journal (
		generation INTEGER PRIMARY KEY,
		outcome TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("initialize Collector activation journal schema: %w", err)
	}
	return nil
}
