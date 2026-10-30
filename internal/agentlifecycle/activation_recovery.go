package agentlifecycle

import (
	"context"
	"fmt"
	"os"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

// pendingActivationRecovery restores the prior config when persisted intent
// shows promotion started but the matching revision never committed.
type pendingActivationRecovery struct {
	store                agentstate.Repository
	lockPath             string
	recover              func(string) error
	restoreLastKnownGood func(string) error
}

// Recover runs before Collector launch and clears intent only after restoring
// the prior config or confirming that the candidate revision committed.
func (r pendingActivationRecovery) Recover(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("activation recovery context is required")
	}
	lock, err := agentstate.AcquireLifecycleLock(r.lockPath)
	if err != nil {
		return fmt.Errorf("acquire lifecycle lock for recovery: %w", err)
	}
	defer lock.Close()

	state, err := r.store.Load()
	if err != nil {
		return fmt.Errorf("load state for activation recovery: %w", err)
	}
	pending := state.PendingActivation
	if pending == nil {
		return nil
	}
	if pending.Generation > state.ActivatedGeneration {
		if r.recover == nil {
			return fmt.Errorf("activation recovery handler is required")
		}
		if err := r.recover(pending.RecoveryPath); err != nil {
			return fmt.Errorf("restore interrupted Collector activation: %w", err)
		}
	}
	if pending.Generation > state.AppliedGeneration &&
		pending.Generation <= state.ActivatedGeneration {
		if pending.Generation != state.ActivatedGeneration || pending.RecoveryPath == "" || r.recover == nil || r.restoreLastKnownGood == nil {
			return fmt.Errorf("interrupted readiness recovery lacks a matching generation or recovery handlers")
		}
		if err := r.recover(pending.RecoveryPath); err != nil {
			return fmt.Errorf("restore applied Collector config after interrupted readiness: %w", err)
		}
		if err := r.restoreLastKnownGood(pending.RecoveryPath); err != nil {
			return fmt.Errorf("restore last-known-good Collector config after interrupted readiness: %w", err)
		}
		_, err = r.store.Update(func(current *agentstate.State) error {
			if current.PendingActivation == nil || current.PendingActivation.Generation != pending.Generation {
				return fmt.Errorf("interrupted activation intent changed during recovery")
			}
			if err := current.RollbackUnappliedActivation(pending.Generation); err != nil {
				return err
			}
			current.PendingActivation = nil
			return nil
		})
		if err != nil {
			return fmt.Errorf("record interrupted readiness rollback: %w", err)
		}
		_ = os.Remove(pending.RecoveryPath)
		return nil
	}
	_, err = r.store.Update(func(current *agentstate.State) error {
		if current.PendingActivation == nil ||
			current.PendingActivation.Generation != pending.Generation {
			return nil
		}
		current.PendingActivation = nil
		return nil
	})
	if err != nil {
		return fmt.Errorf("clear recovered activation intent: %w", err)
	}
	_ = os.Remove(pending.RecoveryPath)
	return nil
}
