package agentstate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	enrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/sqlite"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
)

// Repository loads and atomically updates Agent-owned state. Update serializes
// read-modify-write operations; callers must keep mutations limited to state
// changes and must not perform external I/O inside them.
type Repository interface {
	// Load returns the last committed state or an error when no state exists.
	Load() (State, error)
	// Update atomically applies a state-only mutation and returns the result.
	Update(StateMutation) (State, error)
}

// NewStore creates SQLite operational storage. An explicit legacy .json path is
// imported into a sibling .db file and preserved as migration backup.
func NewStore(path string, initializeSchema func(*sql.DB) error) Repository {
	return NewStoreWithGatewayApplied(path, initializeSchema, nil)
}

// NewStoreWithGatewayApplied creates Agent state storage and binds the
// Configuration-owned Gateway transaction participant at Application
// composition. The callback runs only inside the candidate applied transaction.
func NewStoreWithGatewayApplied(path string, initializeSchema func(*sql.DB) error, markGatewayApplied func(*sql.Tx, uint64, string) error) Repository {
	store := newSQLiteStoreForPath(path, initializeSchema)
	store.markGatewayApplied = markGatewayApplied
	return store
}

// NewIdentityStore returns the SQLite adapter for installation identity at
// the same operational database path used by NewStore.
func NewIdentityStore(path string, initializeSchema func(*sql.DB) error) installationid.InstallationIdentityRepository {
	store := newSQLiteStoreForPath(path, initializeSchema)
	return identitysqlite.New(store.OpenDatabase)
}

// SQLiteUpdateLocker returns the shared process-local write lock used by
// repositories participating in transactions over the Agent database.
func SQLiteUpdateLocker() sync.Locker { return &sqliteUpdateMutex }

// SetDesiredGenerationInTransaction updates Agent-owned desired-generation
// state inside the caller's shared-database transaction.
func SetDesiredGenerationInTransaction(tx *sql.Tx, generation uint64) error {
	state, err := loadSQLiteState(tx)
	if err != nil {
		return err
	}
	state.DesiredGeneration = generation
	return persistGatewayGeneration(tx, state)
}

// MarkGenerationAppliedInTransaction updates Agent-owned activation state and
// its durable candidate journal inside the caller's shared transaction.
func MarkGenerationAppliedInTransaction(tx *sql.Tx, generation uint64) error {
	state, err := loadSQLiteState(tx)
	if err != nil {
		return err
	}
	var candidate *CandidateMapping
	if state.PendingActivation != nil && state.PendingActivation.Generation == generation {
		candidate = state.PendingActivation.Candidate
	}
	if err := state.MarkGenerationApplied(generation); err != nil {
		return err
	}
	content, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode Gateway applied state: %w", err)
	}
	if _, err := tx.Exec(`UPDATE agent_state SET payload = ?, updated_at = ? WHERE id = 1`, content, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist Gateway applied generation: %w", err)
	}
	if candidate != nil {
		mapping := *candidate
		mapping.Outcome, mapping.Stage, mapping.RecoveryResult = "applied", "readiness", "not_needed"
		if err := writeCandidateJournal(tx, generation, mapping); err != nil {
			return err
		}
	}
	return nil
}

func persistGatewayGeneration(tx *sql.Tx, state State) error {
	content, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode Agent state for Core Gateway: %w", err)
	}
	result, err := tx.Exec(`UPDATE agent_state SET payload = ?, updated_at = ? WHERE id = 1`, content, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("persist Agent generation for Core Gateway: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Agent generation writes for Core Gateway: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("persist Agent generation for Core Gateway: expected one state row, got %d", count)
	}
	return nil
}

// NewEnrollmentStore returns the SQLite adapter for durable enrollment attempts
// at the same operational database path used by other Agent state repositories.
func NewEnrollmentStore(path string, initializeSchema func(*sql.DB) error) enrollment.AttemptRepository {
	store := newSQLiteStoreForPath(path, initializeSchema)
	return enrollmentsqlite.New(store.OpenDatabase, &sqliteUpdateMutex)
}

func newSQLiteStoreForPath(path string, initializeSchema func(*sql.DB) error) SQLiteStore {
	path = strings.TrimSpace(path)
	if strings.EqualFold(filepath.Ext(path), ".json") {
		store := NewSQLiteStore(strings.TrimSuffix(path, filepath.Ext(path))+".db", initializeSchema)
		store.legacyPath = path
		return store
	}
	return NewSQLiteStore(path, initializeSchema)
}
