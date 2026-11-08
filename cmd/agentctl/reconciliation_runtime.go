package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation"
)

const reconciliationPollInterval = 15 * time.Second

type runtimeBaseline struct {
	store        agentstate.Repository
	platform     identity.PlatformInfo
	dependencies dependency.Catalog
	layers       []config.Layer
	gateways     coreconfig.GatewayRepository
}

func newRuntimeBaseline(statePath string) (runtimeBaseline, error) {
	store := agentstate.NewStore(statePath)
	state, err := store.Load()
	if err != nil {
		return runtimeBaseline{}, err
	}
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		return runtimeBaseline{}, err
	}
	dependencies, err := defaultDependencyCatalog()
	if err != nil {
		return runtimeBaseline{}, err
	}
	layers, err := configurationLayers(state.Collector.ConfigRoot, platform.OS)
	if err != nil {
		return runtimeBaseline{}, err
	}
	return runtimeBaseline{
		store:        store,
		platform:     platform,
		dependencies: dependencies,
		layers:       layers,
		gateways:     agentstate.NewCoreGatewayStore(statePath),
	}, nil
}

func (b runtimeBaseline) Snapshot() (agentlifecycle.CleanBaselineSnapshot, error) {
	state, err := b.store.Load()
	if err != nil {
		return agentlifecycle.CleanBaselineSnapshot{}, err
	}
	gateway, err := b.gateways.EffectiveGateway()
	if err != nil {
		return agentlifecycle.CleanBaselineSnapshot{}, err
	}
	return agentlifecycle.BuildCleanBaselineSnapshot(b.platform, b.dependencies, b.layers, state, map[string]string{"OTEL_GATEWAY_ENDPOINT": gateway.Endpoint})
}

type runtimeCandidateApplier struct {
	store   agentstate.Repository
	options agentlifecycle.CollectorConfigurationOptions
}

func (a runtimeCandidateApplier) ApplyCandidate(ctx context.Context, request agentlifecycle.CandidateApplyRequest) (agentlifecycle.CandidateActivationOutcome, error) {
	return agentlifecycle.ApplyCollectorCandidate(ctx, a.store, request, a.options)
}

func newRuntimeReconciliationController(statePath string, runner bootstrap.CommandRunner) (*reconciliation.Controller, func(agentstate.State) error, error) {
	baseline, err := newRuntimeBaseline(statePath)
	if err != nil {
		return nil, nil, err
	}
	store := baseline.store
	sqliteStore, ok := store.(agentstate.SQLiteStore)
	if !ok {
		return nil, nil, fmt.Errorf("Agent SQLite store is required for reconciliation")
	}
	state, err := store.Load()
	if err != nil {
		return nil, nil, err
	}
	collectorCatalog, err := collectorruntime.StaticCatalog(baseline.platform)
	if err != nil {
		return nil, nil, err
	}
	registry, err := productionIntegrationRegistry(collectorCatalog)
	if err != nil {
		return nil, nil, err
	}
	resolver := integration.NewResolver(registry)
	options := agentlifecycle.CollectorConfigurationOptions{Platform: baseline.platform, Catalog: baseline.dependencies, Layers: baseline.layers, BinaryPath: state.Collector.BinaryPath, ConfigPath: state.Collector.ConfigPath, ValidationEnvironment: []string{"OTEL_GATEWAY_ENDPOINT=" + state.Collector.GatewayEndpoint}, Runner: runner, GatewayRepository: baseline.gateways}
	attempts := reconciliation.NewSQLiteAttemptsFromStore(sqliteStore)
	controller := &reconciliation.Controller{Desired: sqliteStore.IntegrationRepository(), Baseline: baseline, Resolver: resolver, Compiler: collectorruntime.NewCompiler(collectorCatalog), Validator: func(ctx context.Context, candidate collectorruntime.Candidate) error {
		return collectorruntime.ValidateCandidate(ctx, runner, state.Collector.BinaryPath, candidate, options.ValidationEnvironment)
	}, Applier: runtimeCandidateApplier{store: store, options: options}, Attempts: attempts, Outcomes: reconciliation.CandidateOutcomes{Store: sqliteStore}}
	return controller, attempts.RecordRecoveryFailure, nil
}

func runRuntimeWithReconciliation(ctx context.Context, statePath string, teardown dependencyTeardownFunc, reporter agenthealth.ObservationReporter, runner bootstrap.CommandRunner) error {
	controller, recordRecoveryFailure, err := newRuntimeReconciliationController(statePath, runner)
	if err != nil {
		return fmt.Errorf("compose Agent reconciliation: %w", err)
	}
	inventory := newDockerDiscoveryCoordinator(statePath)
	return agentlifecycle.RunCollectorRuntimeWithReconciliation(
		ctx,
		statePath,
		teardown,
		reporter,
		func(ctx context.Context) error {
			return runRuntimeControlLoops(ctx, func(ctx context.Context) error { return controller.Run(ctx, reconciliationPollInterval) }, inventory)
		},
		func(state agentstate.State, _ error) error {
			return recordRecoveryFailure(state)
		},
	)
}
