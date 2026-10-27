package agentlifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
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
	store agentstate.Repository
	run   collectorRunner

	options      supervisor.Options
	pollInterval time.Duration
	lockPath     string

	applyTeardowns teardownApplicator
	commitConfig   func(string, string) error
	snapshotConfig func(string, uint64) (string, func(), error)
	rollback       func(context.Context, string, uint64) error

	healthReporter agenthealth.ObservationReporter
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
		snapshot, options, cleanupLaunchConfig, err := w.prepareLaunch()
		if err != nil {
			return err
		}

		w.reportHealth(agenthealth.CollectorStateStarting, "")

		launchGeneration := snapshot.ActivatedGeneration
		activeConfigPath := options.ConfigPath
		previousOnReady := options.OnReady
		launchContext, cancelLaunch := context.WithCancel(ctx)

		options.OnReady = func() error {
			if previousOnReady != nil {
				if err := previousOnReady(); err != nil {
					cancelLaunch()
					w.reportHealth(
						agenthealth.CollectorStateFailed,
						err.Error(),
					)

					return err
				}
			}
			if w.commitConfig != nil {
				if err := w.commitConfig(activeConfigPath, options.ConfigPath); err != nil {
					cancelLaunch()
					return fmt.Errorf("commit last-known-good Collector config: %w", err)
				}
			}

			_, err := w.store.Update(func(state *agentstate.State) error {
				return state.MarkGenerationApplied(launchGeneration)
			})
			if err != nil {
				cancelLaunch()

				wrapped := fmt.Errorf(
					"mark Collector generation %d applied: %w",
					launchGeneration,
					err,
				)
				w.reportHealth(
					agenthealth.CollectorStateFailed,
					wrapped.Error(),
				)

				return wrapped
			}

			if w.applyTeardowns != nil {
				if err := w.applyTeardowns(ctx); err != nil {
					cancelLaunch()

					wrapped := fmt.Errorf(
						"apply pending dependency teardowns: %w",
						err,
					)
					w.reportHealth(
						agenthealth.CollectorStateFailed,
						wrapped.Error(),
					)

					return wrapped
				}
			}

			w.reportHealth(agenthealth.CollectorStateReady, "")

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
				cleanupLaunchConfig()

				if err != nil {
					if w.rollback != nil {
						if rollbackErr := w.rollback(ctx, activeConfigPath, launchGeneration); rollbackErr != nil {
							err = fmt.Errorf("rollback failed Collector configuration: %w", rollbackErr)
						}
					}
					w.reportHealth(
						agenthealth.CollectorStateFailed,
						err.Error(),
					)

					return err
				}

				// Collector có thể kết thúc cleanly đúng lúc một generation
				// mới đã được activate. Luôn kiểm tra lại state trước khi
				// kết thúc Agent service.
				if ctx.Err() != nil {
					return nil
				}

				current, loadErr := w.store.Load()
				if loadErr != nil {
					wrapped := fmt.Errorf(
						"reload Agent state after Collector exit: %w",
						loadErr,
					)
					w.reportHealth(
						agenthealth.CollectorStateFailed,
						wrapped.Error(),
					)

					return wrapped
				}

				if current.ActivatedGeneration > launchGeneration {
					restart = true

					continue
				}

				return nil

			case <-ctx.Done():
				ticker.Stop()
				cancelLaunch()
				err := <-runDone
				cleanupLaunchConfig()
				return err

			case <-ticker.C:
				current, err := w.store.Load()
				if err != nil {
					ticker.Stop()
					cancelLaunch()
					<-runDone
					cleanupLaunchConfig()

					wrapped := fmt.Errorf(
						"reload Agent state: %w",
						err,
					)
					w.reportHealth(
						agenthealth.CollectorStateFailed,
						wrapped.Error(),
					)

					return wrapped
				}

				if current.ActivatedGeneration <= launchGeneration {
					continue
				}

				ticker.Stop()
				cancelLaunch()

				if err := <-runDone; err != nil {
					cleanupLaunchConfig()
					wrapped := fmt.Errorf(
						"stop Collector before configuration restart: %w",
						err,
					)
					w.reportHealth(
						agenthealth.CollectorStateFailed,
						wrapped.Error(),
					)

					return wrapped
				}
				cleanupLaunchConfig()

				restart = true
			}
		}
	}
}

// prepareLaunch reads state and snapshots config under the same lifecycle lock
// so the launched bytes match the activated generation.
func (w runtimeWatcher) prepareLaunch() (agentstate.State, supervisor.Options, func(), error) {
	cleanup := func() {}
	if w.lockPath != "" {
		lock, err := agentstate.AcquireLifecycleLock(w.lockPath)
		if err != nil {
			return agentstate.State{}, supervisor.Options{}, cleanup, fmt.Errorf("acquire lifecycle lock for Collector launch: %w", err)
		}
		defer lock.Close()
	}
	snapshot, err := w.store.Load()
	if err != nil {
		return agentstate.State{}, supervisor.Options{}, cleanup, fmt.Errorf("load Agent state: %w", err)
	}
	options := w.options
	if w.snapshotConfig != nil {
		launchConfigPath, remove, err := w.snapshotConfig(options.ConfigPath, snapshot.ActivatedGeneration)
		if err != nil {
			return agentstate.State{}, supervisor.Options{}, cleanup, fmt.Errorf("snapshot Collector config for generation %d: %w", snapshot.ActivatedGeneration, err)
		}
		options.ConfigPath = launchConfigPath
		cleanup = remove
	}
	return snapshot, options, cleanup, nil
}

func (w runtimeWatcher) reportHealth(
	collectorState agenthealth.CollectorState,
	lastError string,
) {
	if w.healthReporter == nil {
		return
	}

	state, err := w.store.Load()
	if err != nil {
		w.healthReporter.Report(agenthealth.RuntimeObservation{
			CollectorState: agenthealth.CollectorStateFailed,
			LastError: fmt.Sprintf(
				"load Agent state for health: %v",
				err,
			),
		})

		return
	}

	pendingTeardowns := 0
	for _, dependency := range state.Dependencies {
		if dependency.PendingTeardown != nil {
			pendingTeardowns++
		}
	}

	w.healthReporter.Report(agenthealth.RuntimeObservation{
		CollectorState:      collectorState,
		DesiredGeneration:   state.DesiredGeneration,
		ActivatedGeneration: state.ActivatedGeneration,
		AppliedGeneration:   state.AppliedGeneration,
		PendingTeardowns:    pendingTeardowns,
		LastError:           lastError,
	})
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
	}
	if err := recovery.Recover(ctx); err != nil {
		return fmt.Errorf("recover interrupted Collector activation: %w", err)
	}

	coordinator := teardownCoordinator{
		store:    store,
		teardown: teardown,
		lockPath: collector.ConfigPath + ".lifecycle",
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
		lockPath:       collector.ConfigPath + ".lifecycle",
		commitConfig:   bootstrap.CommitLastKnownGoodConfigFrom,
		snapshotConfig: bootstrap.SnapshotRuntimeConfig,
		rollback: func(ctx context.Context, configPath string, generation uint64) error {
			return rollbackUnappliedConfiguration(ctx, store, configPath, generation)
		},
		healthReporter: healthReporter,
	}.Run(ctx)
}
