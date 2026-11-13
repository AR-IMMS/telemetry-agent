package agentlifecycle

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

// collectorRunner supervises one Collector process until it exits or its
// context is cancelled.
type collectorRunner func(context.Context, supervisor.Options) error

// teardownApplicator executes safe pending dependency teardowns.
type teardownApplicator func(context.Context) error

type gatewayReadinessVerifier func(context.Context, string, string) error

// runtimeWatcher supervises the Collector and records the configuration
// generation that became healthy at runtime.
type runtimeWatcher struct {
	store          agentstate.Repository
	gatewayRuntime coreconfig.GatewayRuntimeRepository
	run            collectorRunner

	options      supervisor.Options
	pollInterval time.Duration
	lockPath     string

	applyTeardowns         teardownApplicator
	commitConfig           func(string, string) error
	snapshotConfig         func(string, uint64) (string, func(), error)
	rollback               func(context.Context, string, uint64) error
	verifyGatewayReadiness gatewayReadinessVerifier

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
		stableConfigPath := w.options.ConfigPath
		previousOnReady := options.OnReady
		launchContext, cancelLaunch := context.WithCancel(ctx)
		options.OnReady = func() error {
			return w.onCollectorReady(ctx, snapshot, options, launchGeneration, stableConfigPath, previousOnReady, cancelLaunch)
		}

		runDone := make(chan error, 1)
		runFinished := make(chan struct{})

		go func() {
			err := w.run(launchContext, options)
			close(runFinished)
			runDone <- err
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
						if rollbackErr := w.rollback(ctx, stableConfigPath, launchGeneration); rollbackErr != nil {
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
					if err := w.verifyPendingGatewayRevision(
						ctx,
						launchGeneration,
						options,
						runFinished,
					); err != nil {
						ticker.Stop()
						cancelLaunch()
						<-runDone
						cleanupLaunchConfig()
						wrapped := fmt.Errorf("confirm unchanged Core Gateway revision readiness: %w", err)
						w.reportHealth(agenthealth.CollectorStateFailed, wrapped.Error())
						return wrapped
					}
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
	options := w.launchOptions()
	if w.gatewayRuntime != nil {
		gateway, err := w.gatewayRuntime.GatewayForGeneration(snapshot.ActivatedGeneration)
		if err != nil {
			return agentstate.State{}, supervisor.Options{}, cleanup, fmt.Errorf("resolve Gateway endpoint for generation %d: %w", snapshot.ActivatedGeneration, err)
		}
		options.GatewayEndpoint = gateway.Endpoint
	}
	if w.snapshotConfig != nil {
		launchConfigPath, remove, err := w.snapshotConfig(options.ConfigPath, snapshot.ActivatedGeneration)
		if err != nil {
			return agentstate.State{}, supervisor.Options{}, cleanup, fmt.Errorf("snapshot Collector config for generation %d: %w", snapshot.ActivatedGeneration, err)
		}
		options.ConfigPath = launchConfigPath
		cleanup = remove
	}
	if w.snapshotConfig != nil {
		content, err := os.ReadFile(options.ConfigPath)
		if err != nil {
			cleanup()
			return agentstate.State{}, supervisor.Options{}, func() {}, fmt.Errorf("read launch config readiness identity: %w", err)
		}
		options.ReadinessBody, err = config.ReadinessTokenFromConfig(content)
		if err != nil {
			cleanup()
			return agentstate.State{}, supervisor.Options{}, func() {}, err
		}
		if options.ReadinessBody == "" {
			options.ReadinessBody, err = config.NewReadinessToken()
			if err != nil {
				cleanup()
				return agentstate.State{}, supervisor.Options{}, func() {}, err
			}
			content, err = config.WithReadinessToken(content, options.ReadinessBody)
			if err != nil {
				cleanup()
				return agentstate.State{}, supervisor.Options{}, func() {}, err
			}
			if err := os.WriteFile(options.ConfigPath, content, 0o600); err != nil {
				cleanup()
				return agentstate.State{}, supervisor.Options{}, func() {}, fmt.Errorf("bind legacy launch config to readiness generation: %w", err)
			}
		}
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
			ObservedAt:     time.Now().UTC(),
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
		CollectorState:       collectorState,
		DesiredGeneration:    state.DesiredGeneration,
		ActivatedGeneration:  state.ActivatedGeneration,
		AppliedGeneration:    state.AppliedGeneration,
		PendingTeardowns:     pendingTeardowns,
		LastError:            lastError,
		ObservedAt:           time.Now().UTC(),
		SupervisorConfigured: state.Supervisor.Enabled,
	})
}
