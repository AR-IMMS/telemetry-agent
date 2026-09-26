package agentstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxStateBytes = 1 << 20

// FileStore persists Agent state at one machine-local path.
type FileStore struct {
	path string
}

// NewFileStore creates a store for one explicit Agent state file.
func NewFileStore(path string) FileStore {
	return FileStore{
		path: strings.TrimSpace(path),
	}
}

// Save atomically replaces the persisted desired state.
func (s FileStore) Save(state State) error {
	if s.path == "" {
		return fmt.Errorf("Agent state path is required")
	}

	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Agent state: %w", err)
	}

	directory := filepath.Dir(s.path)

	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf(
			"create Agent state directory %q: %w",
			directory,
			err,
		)
	}

	temporaryFile, err := os.CreateTemp(directory, ".agent-state-*")
	if err != nil {
		return fmt.Errorf("create Agent state temporary file: %w", err)
	}

	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if err := temporaryFile.Chmod(0o600); err != nil {
		temporaryFile.Close()

		return fmt.Errorf("set Agent state permissions: %w", err)
	}

	if _, err := temporaryFile.Write(content); err != nil {
		temporaryFile.Close()

		return fmt.Errorf("write Agent state: %w", err)
	}

	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()

		return fmt.Errorf("sync Agent state: %w", err)
	}

	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close Agent state: %w", err)
	}

	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("activate Agent state: %w", err)
	}

	return nil
}

// Load reads the most recently activated Agent state.
func (s FileStore) Load() (State, error) {
	if s.path == "" {
		return State{}, fmt.Errorf("Agent state path is required")
	}

	file, err := os.Open(s.path)
	if err != nil {
		return State{}, fmt.Errorf("open Agent state: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxStateBytes+1))
	if err != nil {
		return State{}, fmt.Errorf("read Agent state: %w", err)
	}

	if len(content) > maxStateBytes {
		return State{}, fmt.Errorf(
			"read Agent state: exceeds %d bytes",
			maxStateBytes,
		)
	}

	var state State

	if err := json.Unmarshal(content, &state); err != nil {
		return State{}, fmt.Errorf("parse Agent state: %w", err)
	}

	return state, nil
}

// StateMutation changes one loaded Agent state while its file is exclusively
// locked from other agentctl processes.
type StateMutation func(*State) error

// Update loads, changes, and atomically saves Agent state while holding an
// operating-system lock for the entire read-modify-write operation.
func (s FileStore) Update(mutate StateMutation) (State, error) {
	if s.path == "" {
		return State{}, fmt.Errorf("Agent state path is required")
	}
	if mutate == nil {
		return State{}, fmt.Errorf("Agent state mutation is required")
	}

	lock, err := acquireStateLock(s.path + ".lock")
	if err != nil {
		return State{}, err
	}
	defer lock.Close()

	state, err := s.Load()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return State{}, err
		}

		state = State{
			Dependencies: make(map[string]DependencyState),
		}
	}

	if err := mutate(&state); err != nil {
		return State{}, err
	}

	if err := s.Save(state); err != nil {
		return State{}, err
	}

	return state, nil
}
