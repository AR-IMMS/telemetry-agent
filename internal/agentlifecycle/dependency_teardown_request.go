package agentlifecycle

import (
	"context"
	"fmt"
	"sort"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// RequestDependencyUninstalls removes the selected dependency receivers from
// desired Collector configuration and activates one shared teardown generation.
func RequestDependencyUninstalls(
	ctx context.Context,
	store agentstate.FileStore,
	options CollectorConfigurationOptions,
	names []string,
) ([]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf("dependency uninstall context is required")
	}

	var changed []string

	_, err := store.Update(func(state *agentstate.State) error {
		var requestErr error

		changed, requestErr = state.RequestUninstalls(names)

		return requestErr
	})
	if err != nil {
		return nil, fmt.Errorf(
			"request dependency uninstalls: %w",
			err,
		)
	}

	if len(changed) == 0 {
		return nil, nil
	}

	if err := ApplyDesiredConfiguration(ctx, store, options); err != nil {
		return nil, fmt.Errorf(
			"activate dependency uninstall configuration: %w",
			err,
		)
	}

	return changed, nil
}

// RequestAllDependencyUninstalls schedules uninstall for every dependency
// currently persisted in Agent state as one desired configuration generation.
func RequestAllDependencyUninstalls(
	ctx context.Context,
	store agentstate.FileStore,
	options CollectorConfigurationOptions,
) ([]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf(
			"dependency teardown request context is required",
		)
	}

	state, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf(
			"load Agent state for dependency teardown request: %w",
			err,
		)
	}

	names := make([]string, 0, len(state.Dependencies))
	for name := range state.Dependencies {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		return nil, nil
	}

	return RequestDependencyUninstalls(
		ctx,
		store,
		options,
		names,
	)
}
