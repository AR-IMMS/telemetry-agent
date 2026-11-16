package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	runtimecontracts "github.com/ar-imms/telemetry-agent/internal/runtime/collector/contracts"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
	"github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

type runtimeBaseline struct {
	store          agentstate.Repository
	platform       identity.PlatformInfo
	dependencies   dependency.Catalog
	layers         []config.Layer
	gateways       coreconfig.GatewayRepository
	profiles       monitoringprofile.Registry
	baselineInputs func() ([]config.InlineLayer, []string, error)
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
	profiles, err := monitoringprofile.LoadDirectory(state.Collector.ConfigRoot)
	if err != nil {
		return runtimeBaseline{}, fmt.Errorf("load local monitoring profiles: %w", err)
	}
	return runtimeBaseline{
		store:        store,
		platform:     platform,
		dependencies: dependencies,
		layers:       layers,
		gateways:     agentstate.NewCoreGatewayStore(statePath),
		profiles:     profiles,
		baselineInputs: func() ([]config.InlineLayer, []string, error) {
			return nativeDockerMonitoringInputsWithProfiles(statePath, platform.OS, profiles)
		},
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
	var additionalLayers []config.InlineLayer
	var additionalReceivers []string
	if b.baselineInputs != nil {
		additionalLayers, additionalReceivers, err = b.baselineInputs()
	}
	if err != nil {
		return reconciliation.BaselineSnapshot{}, fmt.Errorf("resolve native monitoring baseline: %w", err)
	}
	snapshot, err := agentlifecycle.BuildCleanBaselineSnapshotWithAdditionalInputs(b.platform, b.dependencies, b.layers, state, map[string]string{"OTEL_GATEWAY_ENDPOINT": gateway.Endpoint}, additionalLayers, additionalReceivers)
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

type runtimeCandidateCompiler struct{ compiler collectorruntime.Compiler }

func (c runtimeCandidateCompiler) Compile(input config.RenderInput, resolved []integration.ResolvedInstance) (runtimecontracts.Candidate, error) {
	candidate, err := c.compiler.Compile(input, resolved)
	return candidate, classifyRuntimeCandidateError(err)
}

type runtimeCandidateValidator struct {
	runner bootstrap.CommandRunner
	binary string
	env    []string
}

func (v runtimeCandidateValidator) Validate(ctx context.Context, candidate runtimecontracts.Candidate) error {
	return classifyRuntimeCandidateError(collectorruntime.ValidateCandidate(ctx, v.runner, v.binary, candidate, v.env))
}

func classifyRuntimeCandidateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, collectorruntime.ErrUnsupportedCapability) || errors.Is(err, collectorruntime.ErrInvalidIntent) ||
		errors.Is(err, collectorruntime.ErrComponentIDConflict) || errors.Is(err, collectorruntime.ErrInvalidCatalog) ||
		errors.Is(err, collectorruntime.ErrCompilation) || errors.Is(err, collectorruntime.ErrNativeValidation) {
		return errors.Join(runtimecontracts.ErrInvalidCandidate, err)
	}
	return err
}

func (a runtimeCandidateApplier) ApplyCandidate(ctx context.Context, request reconciliation.CandidateApplyRequest) (reconciliation.CandidateActivationOutcome, error) {
	outcome, err := agentlifecycle.ApplyCollectorCandidate(ctx, a.store, request, a.options)
	if errors.Is(err, agentstate.ErrStaleCandidate) {
		return outcome, runtimecontracts.ErrStaleCandidate
	}
	return outcome, err
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
	profiles := baseline.profiles
	additionalBaselineInputs := baseline.baselineInputs
	nativeLayers, _, nativeErr := additionalBaselineInputs()
	if nativeErr != nil {
		return nil, nil, fmt.Errorf("resolve native monitoring ownership: %w", nativeErr)
	}
	resolver := nativeOwnershipResolver{delegate: integration.NewResolver(registry), inventory: sqliteStore.InventoryRepository(), profiles: profiles, enabled: len(nativeLayers) > 0}
	options := agentlifecycle.CollectorConfigurationOptions{Platform: baseline.platform, Catalog: baseline.dependencies, Layers: baseline.layers, AdditionalBaselineInputs: additionalBaselineInputs, BinaryPath: state.Collector.BinaryPath, ConfigPath: state.Collector.ConfigPath, ValidationEnvironment: []string{"OTEL_GATEWAY_ENDPOINT=" + state.Collector.GatewayEndpoint}, Runner: runner, GatewayRepository: baseline.gateways}
	attempts := agentstate.NewSQLiteAttemptsFromStore(sqliteStore)
	controller := &reconciliation.Controller{Desired: sqliteStore.IntegrationRepository(), Baseline: baseline, Resolver: resolver, Compiler: runtimeCandidateCompiler{compiler: collectorruntime.NewCompiler(collectorCatalog)}, Validator: func(ctx context.Context, candidate runtimecontracts.Candidate) error {
		return (runtimeCandidateValidator{runner: runner, binary: state.Collector.BinaryPath, env: options.ValidationEnvironment}).Validate(ctx, candidate)
	}, Applier: runtimeCandidateApplier{store: store, options: options}, Attempts: attempts, Outcomes: attempts}
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
			operationsRunner := newRuntimeOperationsRunner(statePath, reporter)
			var optionalOperations app.OperationsRunner
			if operationsRunner != nil {
				optionalOperations = operationsRunner
			}
			templateMaterializer, err := newRuntimeTemplateMaterializer(statePath, publisher)
			if err != nil {
				return app.RuntimeComponents{}, fmt.Errorf("compose Integration Template materialization: %w", err)
			}
			return app.RuntimeComponents{
				Reconciler: controller, Inventory: inventory,
				RefreshProposals: proposalRefresher.Refresh, RefreshTemplates: templateMaterializer.Refresh,
				RecordRecoveryFailure: func(state agentstate.State, _ error) error { return recordRecoveryFailure(state) },
				Operations:            optionalOperations,
			}, nil
		},
	})
}
