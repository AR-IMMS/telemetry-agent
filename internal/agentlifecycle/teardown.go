package agentlifecycle

import (
	"context"
	"fmt"
	"sort"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// DependencyTeardownFunc routes one safe dependency teardown action to the
// production dependency service.
type DependencyTeardownFunc func(
	context.Context,
	string,
	agentstate.TeardownAction,
	[]agentstate.OwnedResource,
) error

// teardownCoordinator executes teardown actions only after the Collector has
// applied the generation that removes the dependency receiver.
type teardownCoordinator struct {
	store    agentstate.FileStore
	teardown DependencyTeardownFunc
}

// Apply executes every pending teardown currently safe to perform.
func (c teardownCoordinator) Apply(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("teardown context is required")
	}
	if c.teardown == nil {
		return fmt.Errorf("dependency teardown executor is required")
	}

	snapshot, err := c.store.Load()
	if err != nil {
		return fmt.Errorf("load Agent state for teardown: %w", err)
	}

	names := make([]string, 0, len(snapshot.Dependencies))

	for name := range snapshot.Dependencies {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		if err := c.applyDependency(ctx, name); err != nil {
			return err
		}
	}

	return nil
}

func (c teardownCoordinator) applyDependency(
	ctx context.Context,
	name string,
) error {
	_, err := c.store.Update(func(state *agentstate.State) error {
		dependency, exists := state.Dependencies[name]
		if !exists || dependency.PendingTeardown == nil {
			return nil
		}

		pending := dependency.PendingTeardown
		if pending.Generation > state.AppliedGeneration {
			return nil
		}

		if state.OwnsDependency(name) {
			resources := append(
				[]agentstate.OwnedResource(nil),
				dependency.Ownership.Resources...,
			)

			if err := c.teardown(
				ctx,
				name,
				pending.Action,
				resources,
			); err != nil {
				return fmt.Errorf(
					"teardown dependency %q: %w",
					name,
					err,
				)
			}
		}

		if err := state.CompleteTeardown(
			name,
			pending.Generation,
		); err != nil {
			return fmt.Errorf(
				"complete teardown for dependency %q: %w",
				name,
				err,
			)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf(
			"apply pending teardown for dependency %q: %w",
			name,
			err,
		)
	}

	return nil
}
