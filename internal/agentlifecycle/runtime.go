package agentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

const (
	defaultRuntimePollInterval    = 500 * time.Millisecond
	defaultRuntimeStartupTimeout  = 30 * time.Second
	defaultRuntimeShutdownTimeout = 10 * time.Second
)

// RunCollectorRuntime loads the bootstrapped Collector context and supervises
// the Collector until the caller cancels ctx.
func RunCollectorRuntime(
	ctx context.Context,
	statePath string,
	teardown DependencyTeardownFunc,
) error {
	return RunCollectorRuntimeWithHealth(
		ctx,
		statePath,
		teardown,
		nil,
	)
}

// RunCollectorRuntimeWithHealth supervises the Collector and reports its
// transient runtime health to healthReporter.
func RunCollectorRuntimeWithHealth(
	ctx context.Context,
	statePath string,
	teardown DependencyTeardownFunc,
	healthReporter agenthealth.ObservationReporter,
) error {
	return runCollectorRuntimeWithReconciliation(ctx, statePath, teardown, healthReporter, nil, nil)
}

// RunCollectorRuntimeWithReconciliation starts the sole runtime-owned convergence loop after M2 recovery.
func RunCollectorRuntimeWithReconciliation(
	ctx context.Context,
	statePath string,
	teardown DependencyTeardownFunc,
	healthReporter agenthealth.ObservationReporter,
	reconcile func(context.Context) error,
	onRecoveryFailure func(agentstate.State, error) error,
) error {
	return runCollectorRuntimeWithReconciliation(ctx, statePath, teardown, healthReporter, reconcile, onRecoveryFailure)
}

func runCollectorRuntimeWithReconciliation(
	ctx context.Context,
	statePath string,
	teardown DependencyTeardownFunc,
	healthReporter agenthealth.ObservationReporter,
	reconcile func(context.Context) error,
	onRecoveryFailure func(agentstate.State, error) error,
) error {
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return fmt.Errorf("Agent state path is required")
	}

	if teardown == nil {
		return fmt.Errorf(
			"Collector runtime dependency teardown executor is required",
		)
	}

	store := agentstate.NewStore(statePath)

	state, err := store.Load()
	if err != nil {
		return fmt.Errorf(
			"load Collector runtime state: %w",
			err,
		)
	}

	collector := state.Collector
	if strings.TrimSpace(collector.BinaryPath) == "" ||
		strings.TrimSpace(collector.ConfigPath) == "" ||
		strings.TrimSpace(collector.GatewayEndpoint) == "" ||
		strings.TrimSpace(collector.HealthEndpoint) == "" {
		return fmt.Errorf(
			"Collector context is incomplete; bootstrap the Collector before running it",
		)
	}
	recovery := pendingActivationRecovery{
		store:    store,
		lockPath: collector.ConfigPath + ".lifecycle",
		recover: func(path string) error {
			return bootstrap.RestoreConfigArtifact(collector.ConfigPath, path)
		},
		restoreLastKnownGood: func(path string) error {
			return bootstrap.CommitLastKnownGoodConfigFrom(collector.ConfigPath, path)
		},
	}
	if err := recovery.Recover(ctx); err != nil {
		if onRecoveryFailure != nil {
			if recordErr := onRecoveryFailure(state, err); recordErr != nil {
				return errors.Join(fmt.Errorf("recover interrupted Collector activation: %w", err), fmt.Errorf("record reconciliation recovery failure: %w", recordErr))
			}
		}
		return fmt.Errorf("recover interrupted Collector activation: %w", err)
	}
	runtimeContext, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()
	var reconciliationDone chan error
	if reconcile != nil {
		reconciliationDone = make(chan error, 1)
		go func() { reconciliationDone <- reconcile(runtimeContext) }()
	}

	coordinator := teardownCoordinator{
		store:    store,
		teardown: teardown,
		lockPath: collector.ConfigPath + ".lifecycle",
	}
	runtimeRunner := collectorRunner(supervisor.Run)
	if state.Supervisor.Enabled {
		runtimeRunner = func(ctx context.Context, options supervisor.Options) error {
			return runManagedSupervisor(ctx, statePath, state.Supervisor, options)
		}
	}

	runtimeErr := runtimeWatcher{
		store:          store,
		gatewayRuntime: agentstate.NewCoreGatewayStore(statePath),
		run:            runtimeRunner,
		options: supervisor.Options{
			BinaryPath:      collector.BinaryPath,
			ConfigPath:      collector.ConfigPath,
			GatewayEndpoint: collector.GatewayEndpoint,
			HealthEndpoint:  collector.HealthEndpoint,
			StartupTimeout:  defaultRuntimeStartupTimeout,
			ShutdownTimeout: defaultRuntimeShutdownTimeout,
		},
		pollInterval:   defaultRuntimePollInterval,
		applyTeardowns: coordinator.Apply,
		lockPath:       collector.ConfigPath + ".lifecycle",
		commitConfig:   bootstrap.CommitLastKnownGoodConfigFrom,
		snapshotConfig: bootstrap.SnapshotRuntimeConfig,
		rollback: func(ctx context.Context, configPath string, generation uint64) error {
			return rollbackUnappliedConfiguration(ctx, store, configPath, generation)
		},
		healthReporter: healthReporter,
	}.Run(runtimeContext)
	cancelRuntime()
	if reconciliationDone != nil {
		if err := <-reconciliationDone; err != nil && !errors.Is(err, context.Canceled) && runtimeErr == nil {
			return fmt.Errorf("Agent reconciliation service stopped: %w", err)
		}
	}
	return runtimeErr
}
