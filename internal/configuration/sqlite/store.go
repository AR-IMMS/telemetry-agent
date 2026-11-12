package sqlite

import (
	"database/sql"
	"fmt"
	"sync"
)

type database struct {
	open   func() (*sql.DB, error)
	locker sync.Locker
}

func (s database) openDatabase() (*sql.DB, error) {
	if s.open == nil {
		return nil, fmt.Errorf("Configuration SQLite database opener is required")
	}
	return s.open()
}

func (s database) lockWrites() (func(), error) {
	if s.locker == nil {
		return nil, fmt.Errorf("Configuration SQLite write lock is required")
	}
	s.locker.Lock()
	return s.locker.Unlock, nil
}
