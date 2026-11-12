package main

import (
	"context"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation"
)

type runtimeBaseline struct {
	store        agentstate.Repository
	platform     identity.PlatformInfo
	dependencies dependency.Catalog
	layers       []config.Layer
	gateways     coreconfig.GatewayRepository
}

var (
	_ reconciliation.BaselineReader   = runtimeBaseline{}
	_ reconciliation.CandidateApplier = runtimeCandidateApplier{}
)

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

func (b runtimeBaseline) Snapshot() (reconciliation.BaselineSnapshot, error) {
	state, err := b.store.Load()
	if err != nil {
		return reconciliation.BaselineSnapshot{}, err
	}
	gateway, err := b.gateways.EffectiveGateway()
	if err != nil {
		return reconciliation.BaselineSnapshot{}, err
	}
	snapshot, err := agentlifecycle.BuildCleanBaselineSnapshot(b.platform, b.dependencies, b.layers, state, map[string]string{"OTEL_GATEWAY_ENDPOINT": gateway.Endpoint})
	if err != nil {
		return reconciliation.BaselineSnapshot{}, err
	}
	return reconciliation.BaselineSnapshot{
		Input:             snapshot.Input,
		DesiredGeneration: snapshot.DesiredGeneration,
		Fingerprint:       snapshot.Fingerprint,
	}, nil
}

type runtimeCandidateApplier struct {
	store   agentstate.Repository
	options agentlifecycle.CollectorConfigurationOptions
}

func (a runtimeCandidateApplier) ApplyCandidate(ctx context.Context, request reconciliation.CandidateApplyRequest) (reconciliation.CandidateActivationOutcome, error) {
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
	registry, err := productionIntegrationRegistry()
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
	return app.RunRuntime(ctx, app.RuntimeOptions{
		StatePath: statePath,
		Teardown:  teardown,
		Reporter:  reporter,
		Runner:    runner,
		Build: func(publisher events.Publisher, commandRunner bootstrap.CommandRunner) (app.RuntimeComponents, error) {
			controller, recordRecoveryFailure, err := newRuntimeReconciliationController(statePath, commandRunner)
			if err != nil {
				return app.RuntimeComponents{}, fmt.Errorf("compose Agent reconciliation: %w", err)
			}
			inventory := newDockerDiscoveryCoordinator(statePath)
			proposalRefresher, err := newRuntimeProposalRefresher(statePath)
			if err != nil {
				return app.RuntimeComponents{}, fmt.Errorf("compose monitoring proposal refresh: %w", err)
			}
			templateMaterializer, err := newRuntimeTemplateMaterializer(statePath, publisher)
			if err != nil {
				return app.RuntimeComponents{}, fmt.Errorf("compose Integration Template materialization: %w", err)
			}
			return app.RuntimeComponents{
				Reconciler: controller, Inventory: inventory,
				RefreshProposals: proposalRefresher.Refresh, RefreshTemplates: templateMaterializer.Refresh,
				RecordRecoveryFailure: func(state agentstate.State, _ error) error { return recordRecoveryFailure(state) },
			}, nil
		},
	})
}
