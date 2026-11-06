package agentlifecycle

import (
	"context"
	"fmt"
	"os"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func (w runtimeWatcher) onCollectorReady(
	ctx context.Context,
	snapshot agentstate.State,
	options supervisor.Options,
	generation uint64,
	stableConfigPath string,
	previous func() error,
	cancel context.CancelFunc,
) error {
	if previous != nil {
		if err := previous(); err != nil {
			cancel()
			w.reportHealth(agenthealth.CollectorStateFailed, err.Error())
			return err
		}
	}
	if w.commitConfig != nil {
		if err := w.commitConfig(stableConfigPath, options.ConfigPath); err != nil {
			cancel()
			return fmt.Errorf("commit last-known-good Collector config: %w", err)
		}
	}

	var err error
	if w.gatewayRuntime != nil {
		err = w.gatewayRuntime.MarkGatewayApplied(generation, options.GatewayEndpoint)
	} else if candidateStore, ok := w.store.(interface {
		MarkCandidateGenerationApplied(uint64, string) error
	}); ok && snapshot.PendingActivation != nil && snapshot.PendingActivation.Candidate != nil {
		err = candidateStore.MarkCandidateGenerationApplied(generation, options.GatewayEndpoint)
	} else {
		_, err = w.store.Update(func(state *agentstate.State) error {
			return state.MarkGenerationApplied(generation)
		})
	}
	if err != nil {
		cancel()
		wrapped := fmt.Errorf("mark Collector generation %d applied: %w", generation, err)
		w.reportHealth(agenthealth.CollectorStateFailed, wrapped.Error())
		return wrapped
	}
	if snapshot.PendingActivation != nil && snapshot.PendingActivation.RecoveryPath != "" {
		_ = os.Remove(snapshot.PendingActivation.RecoveryPath)
	}
	if w.applyTeardowns != nil {
		if err := w.applyTeardowns(ctx); err != nil {
			cancel()
			wrapped := fmt.Errorf("apply pending dependency teardowns: %w", err)
			w.reportHealth(agenthealth.CollectorStateFailed, wrapped.Error())
			return wrapped
		}
	}
	w.reportHealth(agenthealth.CollectorStateReady, "")
	return nil
}
