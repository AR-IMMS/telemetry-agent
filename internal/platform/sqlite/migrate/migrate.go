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
