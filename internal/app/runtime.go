package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation"
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation"
)

const (
	// ReconciliationPollInterval preserves the existing desired-state fallback cadence.
	ReconciliationPollInterval = 15 * time.Second
	// DiscoveryPollInterval preserves the existing inventory refresh cadence.
	DiscoveryPollInterval     = 30 * time.Second
	eventShutdownTimeout      = 5 * time.Second
	desiredEventQueueCapacity = 32
)

// Reconciler exposes the existing desired-to-runtime convergence entrypoints.
type Reconciler interface {
	Reconcile(context.Context) (reconciliation.Result, error)
	Run(context.Context, time.Duration) error
}

// OperationsRunner is an optional remote read-only command consumer. It must recover independently from broker outages.
type OperationsRunner interface {
	Run(context.Context) error
}

// RuntimeComponents are the executable-built domain adapters the Agent Service coordinates.
type RuntimeComponents struct {
	Reconciler Reconciler
	Inventory  *inventoryctx.Coordinator
	// RefreshProposals refreshes runtime proposal state after inventory evaluation.
	RefreshProposals func(context.Context) error
	// RefreshTemplates projects Templates after a successful inventory cycle.
	RefreshTemplates func(context.Context) error
	// RecordRecoveryFailure persists lifecycle recovery failure details.
	RecordRecoveryFailure func(agentstate.State, error) error
	Operations            OperationsRunner
}

// RuntimeBuilder composes domain/runtime adapters with the app-owned advisory publisher.
type RuntimeBuilder func(events.Publisher, bootstrap.CommandRunner) (RuntimeComponents, error)

// RuntimeOptions provides the stable lifecycle inputs for one Agent Service run.
type RuntimeOptions struct {
	StatePath    string
	Teardown     agentlifecycle.DependencyTeardownFunc
	Reporter     agenthealth.ObservationReporter
	Runner       bootstrap.CommandRunner
	Build        RuntimeBuilder
	StoreFactory agentlifecycle.RuntimeStoreFactory
}

// RunRuntime composes one Agent Service runtime, then starts its sole
// reconciliation loop and optional inventory/proposal/materialization loops.
// All loops share ctx and are stopped when the service lifecycle returns.
func RunRuntime(ctx context.Context, options RuntimeOptions) error {
	if ctx == nil || options.Build == nil || options.StoreFactory == nil {
		return fmt.Errorf("Agent runtime context, builder, and state store factory are required")
	}
	var components RuntimeComponents
	dispatcher, err := events.NewDispatcher(desiredEventQueueCapacity, func(ctx context.Context, _ events.DesiredConfigurationChanged) error {
		_, err := components.Reconciler.Reconcile(ctx)
		if isExpectedReconciliationOutcome(err) {
			return nil
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("compose desired-state event dispatcher: %w", err)
	}
	components, err = options.Build(dispatcher, options.Runner)
	if err != nil {
		return fmt.Errorf("compose Agent runtime dependencies: %w", err)
	}
	if components.Reconciler == nil || components.RecordRecoveryFailure == nil {
		return fmt.Errorf("Agent runtime reconciler and recovery recorder are required")
	}
	return agentlifecycle.RunCollectorRuntimeWithReconciliation(
		ctx,
		options.StatePath,
		options.Teardown,
		options.Reporter,
		func(ctx context.Context) error {
			if err := dispatcher.Start(ctx); err != nil {
				return fmt.Errorf("start desired-state event dispatcher: %w", err)
			}
			defer func() {
				shutdownContext, cancel := context.WithTimeout(context.Background(), eventShutdownTimeout)
				defer cancel()
				if err := dispatcher.Shutdown(shutdownContext); err != nil {
					log.Printf("Desired-state event dispatcher shutdown timed out code=event_shutdown_timeout")
				}
			}()
			return runControlLoops(ctx, components)
		},
		components.RecordRecoveryFailure,
		options.StoreFactory,
	)
}

func runControlLoops(ctx context.Context, components RuntimeComponents) error {
	if ctx == nil || components.Reconciler == nil {
		return fmt.Errorf("Agent control-loop context and reconciler are required")
	}
	if components.Operations == nil {
		return runAgentControlLoops(ctx, components)
	}
	serviceContext, cancel := context.WithCancel(ctx)
	defer cancel()
	controlDone := make(chan error, 1)
	operationsDone := make(chan error, 1)
	go func() { controlDone <- runAgentControlLoops(serviceContext, components) }()
	go func() { operationsDone <- components.Operations.Run(serviceContext) }()
	select {
	case err := <-controlDone:
		cancel()
		operationsErr := <-operationsDone
		if err != nil {
			return err
		}
		return operationsErr
	case err := <-operationsDone:
		cancel()
		controlErr := <-controlDone
		if err != nil {
			return err
		}
		return controlErr
	}
}

func runAgentControlLoops(ctx context.Context, components RuntimeComponents) error {
	if ctx == nil || components.Reconciler == nil {
		return fmt.Errorf("Agent control-loop context and reconciler are required")
	}
	if components.Inventory == nil {
		if components.RefreshProposals != nil {
			if err := components.RefreshProposals(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Proposal refresh failed code=proposal_refresh_error")
			}
		}
		return components.Reconciler.Run(ctx, ReconciliationPollInterval)
	}
	serviceContext, cancel := context.WithCancel(ctx)
	discoveryDone := make(chan struct{})
	go func() {
		defer close(discoveryDone)
		err := components.Inventory.RunWithCycle(serviceContext, DiscoveryPollInterval, func(error) {
			log.Printf("Discovery inventory cycle failed code=inventory_store_error")
		}, func(result inventoryctx.CycleResult) error {
			if components.RefreshTemplates != nil && result.Succeeded() {
				if err := components.RefreshTemplates(serviceContext); err != nil {
					log.Printf("Template materialization refresh failed code=materialization_error")
				}
			}
			if components.RefreshProposals == nil {
				return nil
			}
			return components.RefreshProposals(serviceContext)
		})
		if err != nil && !errors.Is(err, context.Canceled) && serviceContext.Err() == nil {
			log.Printf("Discovery service stopped code=inventory_service_error")
		}
	}()
	err := components.Reconciler.Run(serviceContext, ReconciliationPollInterval)
	cancel()
	<-discoveryDone
	return err
}

func isExpectedReconciliationOutcome(err error) bool {
	return errors.Is(err, reconciliation.ErrAttemptBlocked) || errors.Is(err, reconciliation.ErrAttemptBudgetExhausted) ||
		errors.Is(err, reconciliation.ErrAttemptBackoff) || errors.Is(err, reconciliation.ErrTargetAlreadyApplied) ||
		errors.Is(err, reconciliation.ErrAwaitingLifecycle)
}
