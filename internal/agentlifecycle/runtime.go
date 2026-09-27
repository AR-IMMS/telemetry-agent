package agentlifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

// collectorRunner supervises one Collector process until it exits or its
// context is cancelled.
type collectorRunner func(context.Context, supervisor.Options) error

// teardownApplicator executes safe pending dependency teardowns.
type teardownApplicator func(context.Context) error

// runtimeWatcher supervises the Collector and records the configuration
// generation that became healthy at runtime.
type runtimeWatcher struct {
	store agentstate.FileStore
	run   collectorRunner

	options      supervisor.Options
	pollInterval time.Duration

	applyTeardowns teardownApplicator
}

// Run launches the Collector for the currently activated configuration.
func (w runtimeWatcher) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("runtime watcher context is required")
	}
	if w.run == nil {
		return fmt.Errorf("Collector runtime runner is required")
	}
	if w.pollInterval <= 0 {
		return fmt.Errorf("runtime watcher poll interval must be positive")
	}

	for {
		snapshot, err := w.store.Load()
		if err != nil {
			return fmt.Errorf("load Agent state: %w", err)
		}

		launchGeneration := snapshot.ActivatedGeneration
		options := w.options
		previousOnReady := options.OnReady

		launchContext, cancelLaunch := context.WithCancel(ctx)

		options.OnReady = func() error {
			if previousOnReady != nil {
				if err := previousOnReady(); err != nil {
					cancelLaunch()

					return err
				}
			}

			_, err := w.store.Update(func(state *agentstate.State) error {
				return state.MarkGenerationApplied(launchGeneration)
			})
			if err != nil {
				cancelLaunch()

				return fmt.Errorf(
					"mark Collector generation %d applied: %w",
					launchGeneration,
					err,
				)
			}

			if w.applyTeardowns != nil {
				if err := w.applyTeardowns(ctx); err != nil {
					cancelLaunch()

					return fmt.Errorf(
						"apply pending dependency teardowns: %w",
						err,
					)
				}
			}

			return nil
		}
		runDone := make(chan error, 1)

		go func() {
			runDone <- w.run(launchContext, options)
		}()

		ticker := time.NewTicker(w.pollInterval)
		restart := false

		for !restart {
			select {
			case err := <-runDone:
				ticker.Stop()
				cancelLaunch()

				return err

			case <-ctx.Done():
				ticker.Stop()
				cancelLaunch()

				return <-runDone

			case <-ticker.C:
				current, err := w.store.Load()
				if err != nil {
					ticker.Stop()
					cancelLaunch()
					<-runDone

					return fmt.Errorf("reload Agent state: %w", err)
				}

				if current.ActivatedGeneration <= launchGeneration {
					continue
				}

				ticker.Stop()
				cancelLaunch()

				if err := <-runDone; err != nil {
					return fmt.Errorf(
						"stop Collector before configuration restart: %w",
						err,
					)
				}

				restart = true
			}
		}
	}
}

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
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return fmt.Errorf("Agent state path is required")
	}

	if teardown == nil {
		return fmt.Errorf(
			"Collector runtime dependency teardown executor is required",
		)
	}

	store := agentstate.NewFileStore(statePath)

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

	coordinator := teardownCoordinator{
		store:    store,
		teardown: teardown,
	}

	return runtimeWatcher{
		store: store,
		run:   supervisor.Run,
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
	}.Run(ctx)
}
