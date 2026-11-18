package migrate

import (
	"database/sql"
	"fmt"
	"strings"
)

// Migration describes one globally ordered schema version supplied by its
// semantic owner. Apply retains responsibility for its existing SQL and
// transaction boundary.
type Migration struct {
	// Version is the one-based schema version; registrations must be contiguous.
	Version int
	// Name identifies the migration in validation and error output.
	Name string
	// Apply performs the owner-defined migration. It must preserve the owner's
	// established idempotency and transaction semantics.
	Apply func(*sql.DB) error
}

// InitializeLedger creates the technical migration ledger used by ordered
// migrations. It leaves business tables and migration ordering to composition.
func InitializeLedger(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("SQLite migration database is required")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("initialize SQLite migration ledger: %w", err)
	}
	return nil
}

// RecordApplied records one completed migration in the technical ledger. The
// caller retains responsibility for its transaction boundary and SQL order.
func RecordApplied(db *sql.DB, version int) error {
	if db == nil {
		return fmt.Errorf("SQLite migration database is required")
	}
	if version < 1 {
		return fmt.Errorf("SQLite migration version must be positive")
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP)`, version); err != nil {
		return fmt.Errorf("record SQLite migration version %d: %w", version, err)
	}
	return nil
}

// Apply validates the complete registry, then runs migrations in registration
// order. It does not sort, skip, wrap in transactions, or own domain SQL. The
// first application error stops execution and retains its cause for errors.Is.
func Apply(db *sql.DB, migrations []Migration) error {
	if db == nil {
		return fmt.Errorf("SQLite migration database is required")
	}
	if len(migrations) == 0 {
		return fmt.Errorf("SQLite migrations are required")
	}
	for i, migration := range migrations {
		expectedVersion := i + 1
		if migration.Version != expectedVersion {
			return fmt.Errorf("SQLite migration registry version at index %d is %d, want %d", i, migration.Version, expectedVersion)
		}
		if strings.TrimSpace(migration.Name) == "" {
			return fmt.Errorf("SQLite migration version %d requires a name", migration.Version)
		}
		if migration.Apply == nil {
			return fmt.Errorf("SQLite migration version %d (%s) requires an apply function", migration.Version, migration.Name)
		}
	}
	for _, migration := range migrations {
		if err := migration.Apply(db); err != nil {
			return fmt.Errorf("apply SQLite migration version %d (%s): %w", migration.Version, migration.Name, err)
		}
	}
	return nil
}
