package agentstate

import "errors"

var ErrStateLocked = errors.New(
	"Agent state is already being updated by another process",
)

type stateLock interface {
	Close() error
}
