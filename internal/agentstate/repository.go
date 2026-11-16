package agentstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	coreconfig "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig"
	remoteconfigsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/remoteconfig/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/identity/enrollment"
	enrollmentsqlite "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/sqlite"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation"
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
	return newSQLiteStoreForPath(path, initializeSchema)
}

// NewIdentityStore returns the SQLite adapter for installation identity at
// the same operational database path used by NewStore.
func NewIdentityStore(path string, initializeSchema func(*sql.DB) error) installationid.InstallationIdentityRepository {
	store := newSQLiteStoreForPath(path, initializeSchema)
	return identitysqlite.New(store.OpenDatabase)
}

// NewCoreGatewayStore returns the SQLite adapter for Core Gateway snapshot
// acceptance at the same operational database path used by NewStore.
func NewCoreGatewayStore(path string, initializeSchema func(*sql.DB) error) coreconfig.GatewayRuntimeRepository {
	store := newSQLiteStoreForPath(path, initializeSchema)
	return newCoreGatewayRepository(store)
}

func newCoreGatewayRepository(store SQLiteStore) coreconfig.GatewayRuntimeRepository {
	return remoteconfigsqlite.New(func(ctx context.Context) (*sql.DB, error) { return store.OpenDatabaseContext(ctx) }, &sqliteUpdateMutex, remoteconfigsqlite.StateAccess{
		BeginSnapshotAcceptance: func() (func(), error) {
			state, err := store.Load()
			if err != nil {
				return nil, fmt.Errorf("load Agent state before Core Gateway acceptance: %w", err)
			}
			if state.Collector.ConfigPath == "" {
				return nil, nil
			}
			lifecycleLock, err := AcquireLifecycleLock(state.Collector.ConfigPath + ".lifecycle")
			if err != nil {
				return nil, fmt.Errorf("acquire lifecycle lock for Core Gateway acceptance: %w", err)
			}
			return func() { _ = lifecycleLock.Close() }, nil
		},
		ReadInstallationID: func(tx *sql.Tx) (string, bool, error) {
			id, initialized, err := identitysqlite.ReadInstallationID(tx)
			return id.String(), initialized, err
		},
		Read: func(tx *sql.Tx) (remoteconfigsqlite.AgentState, error) {
			state, err := loadSQLiteState(tx)
			if err != nil {
				return remoteconfigsqlite.AgentState{}, err
			}
			return remoteconfigsqlite.AgentState{
				BootstrapEndpoint:   state.Collector.GatewayEndpoint,
				DesiredGeneration:   state.DesiredGeneration,
				ActivatedGeneration: state.ActivatedGeneration,
				AppliedGeneration:   state.AppliedGeneration,
			}, nil
		},
		SetDesiredGeneration: func(tx *sql.Tx, generation uint64) error {
			state, err := loadSQLiteState(tx)
			if err != nil {
				return err
			}
			state.DesiredGeneration = generation
			return persistGatewayGeneration(tx, state)
		},
		MarkGenerationApplied: func(tx *sql.Tx, generation uint64) error {
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
		},
	})
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
