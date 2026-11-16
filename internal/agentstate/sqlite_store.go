package agentstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	platformsqlite "github.com/ar-imms/telemetry-agent/internal/platform/sqlite"
)

// sqliteUpdateMutex serializes in-process mutations across store values.
// SQLite's immediate transactions and busy timeout coordinate other processes.
var sqliteUpdateMutex sync.Mutex

// SQLiteStore persists Agent operational state in one local SQLite database.
// Each operation owns and closes its database handle; transactions remain
// short and callbacks must contain state changes only.
type SQLiteStore struct {
	path               string
	legacyPath         string
	initializeSchema   func(*sql.DB) error
	markGatewayApplied func(*sql.Tx, uint64, string) error
}

// NewSQLiteStore creates a SQLite WAL store at the supplied database path.
// Application composition supplies its ordered schema initializer explicitly.
func NewSQLiteStore(path string, initializeSchema func(*sql.DB) error) SQLiteStore {
	return NewSQLiteStoreWithGatewayApplied(path, initializeSchema, nil)
}

// NewSQLiteStoreWithGatewayApplied creates a SQLite store whose candidate
// completion can enlist Configuration's Gateway attribution in the same SQL
// transaction. Application composition supplies the owner callback.
func NewSQLiteStoreWithGatewayApplied(path string, initializeSchema func(*sql.DB) error, markGatewayApplied func(*sql.Tx, uint64, string) error) SQLiteStore {
	path = strings.TrimSpace(path)
	legacy := filepath.Join(filepath.Dir(path), "state.json")
	return SQLiteStore{path: path, legacyPath: legacy, initializeSchema: initializeSchema, markGatewayApplied: markGatewayApplied}
}

// OpenDatabase opens the operational database after initializing its shared schema.
func (s SQLiteStore) OpenDatabase() (*sql.DB, error) {
	return s.openWithContext(context.Background())
}

// OpenDatabaseContext opens the operational database under the caller context.
func (s SQLiteStore) OpenDatabaseContext(ctx context.Context) (*sql.DB, error) {
	return s.openWithContext(ctx)
}

// Save atomically replaces the persisted Agent state.
func (s SQLiteStore) Save(state State) error {
	_, err := s.Update(func(current *State) error {
		*current = state
		return nil
	})
	return err
}

// Load reads the most recently committed Agent state.
func (s SQLiteStore) Load() (State, error) {
	db, err := s.open()
	if err != nil {
		return State{}, err
	}
	defer db.Close()
	return loadSQLiteState(db)
}

// Update serializes a state-only read-modify-write transaction.
func (s SQLiteStore) Update(mutate StateMutation) (State, error) {
	if mutate == nil {
		return State{}, fmt.Errorf("Agent state mutation is required")
	}
	sqliteUpdateMutex.Lock()
	defer sqliteUpdateMutex.Unlock()

	db, err := s.open()
	if err != nil {
		return State{}, err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return State{}, fmt.Errorf("begin Agent state transaction: %w", err)
	}
	defer tx.Rollback()

	state, err := loadSQLiteState(tx)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return State{}, err
		}
		state = State{Dependencies: make(map[string]DependencyState)}
	}
	previous := state
	if err := mutate(&state); err != nil {
		return State{}, err
	}
	if previous.PendingActivation != nil && previous.PendingActivation.Candidate != nil && state.AppliedGeneration > previous.AppliedGeneration {
		return State{}, ErrCandidateTerminalRequired
	}
	content, err := json.Marshal(state)
	if err != nil {
		return State{}, fmt.Errorf("encode Agent state: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO agent_state (id, payload, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`,
		content,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return State{}, fmt.Errorf("persist Agent state: %w", err)
	}
	if err := recordActivationTransition(tx, previous, state); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, fmt.Errorf("commit Agent state transaction: %w", err)
	}
	return state, nil
}

func (s SQLiteStore) open() (*sql.DB, error) {
	return s.openWithContext(context.Background())
}

func (s SQLiteStore) openWithContext(ctx context.Context) (*sql.DB, error) {
	if s.path == "" {
		return nil, fmt.Errorf("Agent SQLite database path is required")
	}
	if s.initializeSchema == nil {
		return nil, fmt.Errorf("Agent SQLite schema initializer is required")
	}
	return platformsqlite.Open(ctx, s.path, func(db *sql.DB) error {
		if err := s.initializeSchema(db); err != nil {
			return err
		}
		return s.importLegacyState(db)
	})
}

type stateQueryer interface {
	QueryRow(string, ...any) *sql.Row
}

func loadSQLiteState(queryer stateQueryer) (State, error) {
	var content []byte
	if err := queryer.QueryRow(
		`SELECT payload FROM agent_state WHERE id = 1`,
	).Scan(&content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return State{}, fmt.Errorf("load Agent state: %w", os.ErrNotExist)
		}
		return State{}, fmt.Errorf("load Agent state from SQLite: %w", err)
	}
	var state State
	if err := json.Unmarshal(content, &state); err != nil {
		return State{}, fmt.Errorf("decode Agent state from SQLite: %w", err)
	}
	return state, nil
}
