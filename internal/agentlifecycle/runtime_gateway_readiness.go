package agentlifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

// verifyPendingGatewayRevision advances an unchanged Core revision only after
// the supervised Collector still answers with its active generation token.
func (w runtimeWatcher) verifyPendingGatewayRevision(
	ctx context.Context,
	generation uint64,
	options supervisor.Options,
	runFinished <-chan struct{},
) error {
	if w.gatewayRuntime == nil {
		return nil
	}
	gateway, err := w.gatewayRuntime.GatewayForGeneration(generation)
	if err != nil {
		return fmt.Errorf("load Gateway readiness state: %w", err)
	}
	if !gateway.NeedsReadiness || gateway.Endpoint != options.GatewayEndpoint {
		return nil
	}
	select {
	case <-runFinished:
		return nil
	default:
	}
	verify := w.verifyGatewayReadiness
	if verify == nil {
		verify = supervisor.VerifyReadiness
	}
	probeContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := verify(probeContext, options.HealthEndpoint, options.ReadinessBody); err != nil {
		// A failed probe leaves the accepted revision pending for the next poll.
		return nil
	}
	select {
	case <-runFinished:
		return nil
	default:
	}
	if err := w.gatewayRuntime.MarkGatewayApplied(generation, options.GatewayEndpoint); err != nil {
		return fmt.Errorf("commit readiness-confirmed Core Gateway revision: %w", err)
	}
	return nil
}
