package agentlifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// WaitForDependenciesRemoved waits until every dependency record has been
// removed from Agent state by the runtime teardown coordinator.
func WaitForDependenciesRemoved(
	ctx context.Context,
	store agentstate.FileStore,
	pollInterval time.Duration,
) (agentstate.State, error) {
	if ctx == nil {
		return agentstate.State{}, fmt.Errorf(
			"dependency teardown wait context is required",
		)
	}
	if pollInterval <= 0 {
		return agentstate.State{}, fmt.Errorf(
			"dependency teardown wait poll interval must be positive",
		)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		state, err := store.Load()
		if err != nil {
			return agentstate.State{}, fmt.Errorf(
				"load Agent state while waiting for dependency teardowns: %w",
				err,
			)
		}
		if len(state.Dependencies) == 0 {
			return state, nil
		}

		select {
		case <-ctx.Done():
			return agentstate.State{}, fmt.Errorf(
				"wait for dependency teardowns: %w",
				ctx.Err(),
			)
		case <-ticker.C:
		}
	}
}
