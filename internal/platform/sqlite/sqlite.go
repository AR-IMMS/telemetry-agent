package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var initializationMu sync.Mutex
var updateMu sync.Mutex

// UpdateLocker returns the process-wide lock for coordinated SQLite mutations.
// Repositories sharing the Agent database use this lock only around short
// transactions; SQLite still coordinates writers across processes.
func UpdateLocker() sync.Locker { return &updateMu }

// Open creates a single-connection SQLite handle with the Agent's established
// durability and locking settings, then calls initialize before returning it.
// The caller owns closing the returned handle. Initialization failure closes
// the handle and returns the initialization error with context.
func Open(ctx context.Context, path string, initialize func(*sql.DB) error) (*sql.DB, error) {
	if ctx == nil {
		return nil, fmt.Errorf("SQLite open context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("SQLite database path is required")
	}
	if initialize == nil {
		return nil, fmt.Errorf("SQLite initialization callback is required")
	}

	initializationMu.Lock()
	defer initializationMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create Agent database directory: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open Agent SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to Agent SQLite database: %w", err)
	}
	if err := initialize(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
