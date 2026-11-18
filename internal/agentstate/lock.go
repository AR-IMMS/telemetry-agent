package agentstate

import (
	"errors"

	"github.com/ar-imms/telemetry-agent/internal/platform/filelock"
)

// ErrStateLocked indicates that another process is updating the Agent state.
var ErrStateLocked = errors.New(
	"Agent state is already being updated by another process",
)

type stateLock = filelock.Lock

func acquireStateLock(path string) (stateLock, error) {
	lock, err := filelock.Acquire(path)
	if errors.Is(err, filelock.ErrLocked) {
		return nil, ErrStateLocked
	}
	return lock, err
}
