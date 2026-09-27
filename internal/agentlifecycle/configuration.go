package agentlifecycle

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// configurationActivator renders, validates, and atomically activates one
// Collector configuration for the supplied desired state snapshot.
type configurationActivator func(
	context.Context,
	agentstate.State,
) error

// configurationCoordinator activates desired Collector configuration and then
// records the exact state generation that was activated.
type configurationCoordinator struct {
	store    agentstate.FileStore
	activate configurationActivator
}

// Apply activates the current desired configuration before acknowledging its
// generation as activated in persistent state.
func (c configurationCoordinator) Apply(ctx context.Context) error {
	snapshot, err := c.store.Load()
	if err != nil {
		return fmt.Errorf("load Agent state: %w", err)
	}

	if err := c.activate(ctx, snapshot); err != nil {
		return fmt.Errorf("activate desired Collector configuration: %w", err)
	}

	_, err = c.store.Update(func(state *agentstate.State) error {
		return state.MarkGenerationActivated(
			snapshot.DesiredGeneration,
		)
	})
	if err != nil {
		return fmt.Errorf(
			"mark desired Collector configuration activated: %w",
			err,
		)
	}

	return nil
}
