package agentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// ErrStaleConfiguration indicates desired state changed while a candidate was
// being activated.
var ErrStaleConfiguration = errors.New("configuration plan is stale")

// activationMutex prevents two lifecycle entry points from activating
// different candidates concurrently in the same Agent process.
var activationMutex sync.Mutex

// configurationActivator renders, validates, and atomically activates one
// Collector configuration for the supplied desired state snapshot.
type configurationActivator func(
	context.Context,
	agentstate.State,
) error

// configurationCoordinator activates desired Collector configuration and then
// records the exact state generation that was activated.
type configurationCoordinator struct {
	store    agentstate.Repository
	activate configurationActivator
	rollback func() error
	lockPath string
	prepare  func(agentstate.State) (string, error)
	recover  func(string) error
}

// Apply activates the current desired configuration before acknowledging its
// generation in state. The dedicated OS lock spans promotion so another Agent
// process cannot overwrite or roll back the active artifact concurrently.
func (c configurationCoordinator) Apply(ctx context.Context) error {
	activationMutex.Lock()
	defer activationMutex.Unlock()
	if c.lockPath != "" {
		lock, err := agentstate.AcquireLifecycleLock(c.lockPath)
		if err != nil {
			return fmt.Errorf("acquire Agent lifecycle lock: %w", err)
		}
		defer lock.Close()
	}

	// Keep the previous contract: applying configuration requires persisted
	// Agent state and must not create an empty state file.
	snapshot, err := c.store.Load()
	if err != nil {
		return fmt.Errorf("load Agent state: %w", err)
	}
	if snapshot.ActivatedGeneration == snapshot.DesiredGeneration {
		return nil
	}
	plan := newConfigurationPlan(snapshot)
	recoveryPath := ""
	if c.prepare != nil {
		recoveryPath, err = c.prepare(plan.snapshot)
		if err != nil {
			return fmt.Errorf("prepare configuration recovery artifact: %w", err)
		}
		_, err = c.store.Update(func(state *agentstate.State) error {
			if state.DesiredGeneration != plan.revision {
				return ErrStaleConfiguration
			}
			return state.BeginActivation(plan.revision, recoveryPath)
		})
		if err != nil {
			return fmt.Errorf("persist configuration activation intent: %w", err)
		}
	}

	if err := c.activate(ctx, plan.snapshot); err != nil {
		if recoveryErr := c.recoverCandidate(recoveryPath); recoveryErr != nil {
			return fmt.Errorf("activation failed: %v; restore previous config: %w", err, recoveryErr)
		}
		if clearErr := c.clearActivationIntent(plan.revision); clearErr != nil {
			return fmt.Errorf("activation failed: %v; clear recovery intent: %w", err, clearErr)
		}
		_ = os.Remove(recoveryPath)
		return fmt.Errorf(
			"activate desired Collector configuration: %w",
			err,
		)
	}

	_, err = c.store.Update(func(state *agentstate.State) error {
		if state.DesiredGeneration != plan.revision {
			return ErrStaleConfiguration
		}
		if err := state.MarkGenerationActivated(
			plan.revision,
		); err != nil {
			return fmt.Errorf(
				"mark desired Collector configuration activated: %w",
				err,
			)
		}
		if recoveryPath != "" {
			if err := state.CompleteActivation(plan.revision); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		if recoveryPath != "" || (errors.Is(err, ErrStaleConfiguration) && c.rollback != nil) {
			var rollbackErr error
			if recoveryPath != "" {
				rollbackErr = c.recoverCandidate(recoveryPath)
			} else {
				rollbackErr = c.rollback()
			}
			if rollbackErr != nil {
				return fmt.Errorf(
					"configuration commit failed and rollback failed: %w",
					rollbackErr,
				)
			}
			if clearErr := c.clearActivationIntent(plan.revision); clearErr != nil {
				return fmt.Errorf("configuration commit failed: %v; clear recovery intent: %w", err, clearErr)
			}
			_ = os.Remove(recoveryPath)
		}
		return err
	}
	_ = os.Remove(recoveryPath)

	return nil
}

func (c configurationCoordinator) recoverCandidate(path string) error {
	if path == "" || c.recover == nil {
		return nil
	}
	return c.recover(path)
}

func (c configurationCoordinator) clearActivationIntent(generation uint64) error {
	_, err := c.store.Update(func(state *agentstate.State) error {
		if state.PendingActivation != nil && state.PendingActivation.Generation == generation {
			state.PendingActivation = nil
		}
		return nil
	})
	return err
}
