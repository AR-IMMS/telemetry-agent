package supervisor

import "fmt"

// State identifies the externally meaningful phase of Collector supervision.
type State string

const (
	// StateStarting means the Collector has not yet passed readiness.
	StateStarting State = "starting"
	// StateReady means the Collector passed its health check.
	StateReady State = "ready"
	// StateStopping means graceful shutdown has been requested.
	StateStopping State = "stopping"
	// StateStopped means the Collector exited after a controlled shutdown.
	StateStopped State = "stopped"
	// StateFailed means startup, readiness, or shutdown did not complete normally.
	StateFailed State = "failed"
)

type lifecycle struct {
	state State
}

func newLifecycle() *lifecycle {
	// Every run begins before readiness; callers must explicitly advance state.
	return &lifecycle{
		state: StateStarting,
	}
}

func (l *lifecycle) State() State {
	// Expose the current state without allowing callers to mutate it directly.
	return l.state
}

func (l *lifecycle) Transition(next State) error {
	// Reject impossible transitions so callers cannot report a stopped process as ready.
	if !canTransition(l.state, next) {
		return fmt.Errorf(
			"invalid supervisor lifecycle transition from %q to %q",
			l.state,
			next,
		)
	}

	l.state = next

	return nil
}

func canTransition(current State, next State) bool {
	// Keep the state machine finite and one-way; retries belong outside lifecycle state.
	switch current {
	case StateStarting:
		return next == StateReady ||
			next == StateStopping ||
			next == StateFailed

	case StateReady:
		return next == StateStopping ||
			next == StateFailed

	case StateStopping:
		return next == StateStopped ||
			next == StateFailed

	default:
		return false
	}
}
