package agentstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	sqlitemigrate "github.com/ar-imms/telemetry-agent/internal/platform/sqlite/migrate"
)

// ensureSQLiteSchema registers the existing schema versions with the technical
// coordinator before any state access. The migration functions retain their
// SQL and transaction ownership while context adapters are extracted.
func ensureSQLiteSchema(db *sql.DB) error {
	return sqlitemigrate.Apply(db, []sqlitemigrate.Migration{
		{Version: 1, Name: "Agent state and activation journal", Apply: initializeBaseSchema},
		{Version: 2, Name: "Installation identity", Apply: migrateInstallationIdentity},
		{Version: 3, Name: "Core Gateway snapshot", Apply: migrateCoreGatewaySnapshot},
		{Version: 4, Name: "Core Gateway runtime", Apply: migrateCoreGatewayRuntime},
		{Version: 5, Name: "Enrollment attempts", Apply: migrateEnrollmentAttempts},
		{Version: 6, Name: "Integration desired state", Apply: migrateIntegrationDesiredState},
		{Version: 7, Name: "Candidate activation mapping", Apply: migrateCandidateActivationMapping},
		{Version: 8, Name: "Candidate attempt history", Apply: migrateCandidateAttemptHistory},
		{Version: 9, Name: "Reconciliation diagnostics", Apply: migrateReconciliationDiagnostics},
		{Version: 10, Name: "Discovery inventory", Apply: migrateDiscoveryInventory},
		{Version: 11, Name: "Inventory write revision", Apply: migrateInventoryWriteRevision},
		{Version: 12, Name: "Monitoring proposals", Apply: migrateMonitoringProposals},
		{Version: 13, Name: "Integration templates", Apply: migrateIntegrationTemplates},
		{Version: 14, Name: "Operations execution and outbox", Apply: migrateOperationsExecution},
	})
}

func initializeBaseSchema(db *sql.DB) error {
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

// migrateInstallationIdentity atomically creates the identity schema and its
// explicit uninitialized singleton row, independently of UUID initialization.
func migrateInstallationIdentity(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Agent identity schema migration: %w", err)
	}
	defer tx.Rollback()

	var applied int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE version = 2`,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check Agent identity schema version: %w", err)
	}
	if applied != 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("finish Agent identity schema check: %w", err)
		}
		return nil
	}

	var tableExists int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'installation_identity'`,
	).Scan(&tableExists); err != nil {
		return fmt.Errorf("check Agent identity table: %w", err)
	}
	if tableExists != 0 {
		return fmt.Errorf("Agent identity table exists without schema migration version 2")
	}
	if _, err := tx.Exec(
		`CREATE TABLE installation_identity (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			installation_id TEXT,
			initialized INTEGER NOT NULL CHECK (initialized IN (0, 1)),
			CHECK (
				(initialized = 0 AND installation_id IS NULL) OR
				(initialized = 1 AND installation_id IS NOT NULL)
			)
		)`,
	); err != nil {
		return fmt.Errorf("create Agent identity table: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO installation_identity (id, installation_id, initialized)
		 VALUES (1, NULL, 0)`,
	); err != nil {
		return fmt.Errorf("initialize Agent identity row: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, applied_at) VALUES (2, CURRENT_TIMESTAMP)`,
	); err != nil {
		return fmt.Errorf("record Agent identity schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Agent identity schema migration: %w", err)
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
	var candidate *CandidateMapping
	if current.PendingActivation != nil && current.PendingActivation.Generation == generation {
		candidate = current.PendingActivation.Candidate
	}
	if candidate == nil && previous.PendingActivation != nil && previous.PendingActivation.Generation == generation {
		candidate = previous.PendingActivation.Candidate
	}
	if candidate != nil {
		mapping := *candidate
		mapping.Outcome = outcome
		if outcome == "activated" {
			mapping.Stage = "activation"
		} else {
			mapping.Stage = "rollback"
			mapping.RecoveryResult = "recovered"
		}
		return writeCandidateJournal(tx, generation, mapping)
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
