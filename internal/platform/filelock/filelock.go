package filelock

import (
	"errors"
	"fmt"
	"strings"
)

// ErrPathRequired indicates that Acquire received no lock-file path.
var ErrPathRequired = errors.New("lock file path is required")

// ErrLocked indicates that another process currently owns the lock.
var ErrLocked = errors.New("file is already locked by another process")

// Lock holds an exclusive operating-system file lock until Close succeeds.
type Lock interface {
	Close() error
}

// Acquire exclusively locks the exact file at path, creating it if needed.
// The lock is released by closing the returned handle. Locking blocks where
// the operating system primitive blocks; callers own timeout and path policy.
func Acquire(path string) (Lock, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrPathRequired
	}
	lock, err := acquire(path)
	if err != nil {
		return nil, fmt.Errorf("acquire file lock %q: %w", path, err)
	}
	return lock, nil
}
