package agentstate

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LifecycleLock coordinates configuration and managed-resource side effects
// across Agent CLI and runtime processes. It is separate from DB transactions.
type LifecycleLock interface {
	Close() error
}

// AcquireLifecycleLock acquires a cross-process lock beside the config path.
func AcquireLifecycleLock(path string) (LifecycleLock, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("lifecycle lock path is required")
	}
	lockPath := filepath.Clean(path) + ".lock"
	lock, err := acquireStateLock(lockPath)
	if err != nil {
		return nil, err
	}
	return lock, nil
}
