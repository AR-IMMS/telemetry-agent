package agentlifecycle

import (
	"context"
	"fmt"
	"os"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
)

func rollbackUnappliedConfiguration(
	ctx context.Context,
	store agentstate.Repository,
	configPath string,
	generation uint64,
) error {
	activationMutex.Lock()
	defer activationMutex.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	state, err := store.Load()
	if err != nil {
		return fmt.Errorf("load failed activation state: %w", err)
	}
	lock, err := agentstate.AcquireLifecycleLock(configPath + ".lifecycle")
	if err != nil {
		return fmt.Errorf("acquire lifecycle lock for rollback: %w", err)
	}
	defer lock.Close()
	state, err = store.Load()
	if err != nil {
		return fmt.Errorf("reload failed activation state: %w", err)
	}
	if state.ActivatedGeneration != generation ||
		state.AppliedGeneration >= generation {
		return nil
	}
	recoveryPath := ""
	if state.PendingActivation != nil && state.PendingActivation.Generation == generation {
		recoveryPath = state.PendingActivation.RecoveryPath
	}
	if recoveryPath != "" {
		if err := bootstrap.RestoreConfigArtifact(configPath, recoveryPath); err != nil {
			return err
		}
		if err := bootstrap.CommitLastKnownGoodConfigFrom(configPath, recoveryPath); err != nil {
			return err
		}
	} else if err := bootstrap.RestoreLastKnownGoodConfig(configPath); err != nil {
		return err
	}
	_, err = store.Update(func(current *agentstate.State) error {
		if current.ActivatedGeneration != generation ||
			current.AppliedGeneration >= generation {
			return nil
		}
		if err := current.RollbackUnappliedActivation(generation); err != nil {
			return err
		}
		if current.PendingActivation != nil && current.PendingActivation.Generation == generation {
			current.PendingActivation = nil
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("record activation rollback: %w", err)
	}
	if recoveryPath != "" {
		_ = os.Remove(recoveryPath)
	}
	return nil
}
