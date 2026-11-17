package lifecyclelock

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/platform/filelock"
)

// Lock is the file-lock handle returned by Acquire.
type Lock = filelock.Lock

// Acquire locks the normalized config path's adjacent .lock file until Close.
func Acquire(configPath string) (Lock, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return nil, fmt.Errorf("lifecycle lock path is required")
	}
	lockPath := filepath.Clean(configPath) + ".lock"
	lock, err := filelock.Acquire(lockPath)
	if err != nil {
		return nil, fmt.Errorf("acquire lifecycle lock: %w", err)
	}
	return lock, nil
}
