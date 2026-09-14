package supervisor

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultReadinessPollInterval = 500 * time.Millisecond

// Options defines the Collector process, health endpoint, gateway endpoint,
// and bounded startup/shutdown lifecycle windows.
type Options struct {
	BinaryPath      string
	ConfigPath      string
	HealthEndpoint  string
	GatewayEndpoint string
	StartupTimeout  time.Duration
	ShutdownTimeout time.Duration
}

// Run starts and supervises one Collector process until it exits or ctx is
// cancelled.
func Run(ctx context.Context, options Options) error {
	// Production wiring uses the OS process starter and default HTTP transport;
	// runWith supplies the same orchestration with injectable boundaries for tests.
	return runWith(ctx, options, newExecStarter(), http.DefaultClient)
}

func runWith(
	ctx context.Context,
	options Options,
	starter Starter,
	client *http.Client,
) error {
	// Keep process creation and HTTP dependencies injectable for deterministic tests
	// without changing the production supervision path.
	if err := options.validate(); err != nil {
		return err
	}

	if client == nil {
		// Readiness is mandatory because process start alone does not prove Collector health.
		return fmt.Errorf("Collector readiness HTTP client is required")
	}

	readiness := func(readinessCtx context.Context) error {
		startupCtx, cancel := context.WithTimeout(
			readinessCtx,
			options.StartupTimeout,
		)
		defer cancel()

		return waitForReadiness(
			startupCtx,
			client,
			options.HealthEndpoint,
			defaultReadinessPollInterval,
		)
	}

	return newSupervisor(
		starter,
		readiness,
		options.ShutdownTimeout,
	).Run(ctx, LaunchOptions{
		BinaryPath: options.BinaryPath,
		ConfigPath: options.ConfigPath,
		Environment: []string{
			"OTEL_GATEWAY_ENDPOINT=" +
				strings.TrimSpace(options.GatewayEndpoint),
		},
	})
}

func (o Options) validate() error {
	// Validate the health URL here so readiness cannot silently target an invalid scheme.
	if strings.TrimSpace(o.BinaryPath) == "" {
		return fmt.Errorf("Collector binary path is required")
	}
	if strings.TrimSpace(o.ConfigPath) == "" {
		return fmt.Errorf("Collector configuration path is required")
	}
	if o.StartupTimeout <= 0 {
		return fmt.Errorf("Collector startup timeout must be positive")
	}
	if o.ShutdownTimeout <= 0 {
		return fmt.Errorf("Collector shutdown timeout must be positive")
	}

	endpoint, err := url.ParseRequestURI(o.HealthEndpoint)

	if strings.TrimSpace(o.GatewayEndpoint) == "" {
		return fmt.Errorf("Collector gateway endpoint is required")
	}

	if err != nil ||
		endpoint.Scheme == "" ||
		endpoint.Host == "" {
		// Require an absolute endpoint so health checks cannot silently use a relative URL.
		return fmt.Errorf(
			"Collector health endpoint must be an absolute HTTP URL",
		)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		// Limit readiness traffic to the supported HTTP transports.
		return fmt.Errorf(
			"Collector health endpoint scheme %q is unsupported",
			endpoint.Scheme,
		)
	}

	return nil
}

type readinessWaiter func(context.Context) error

type supervisor struct {
	starter          Starter
	waitForReadiness readinessWaiter
	shutdownTimeout  time.Duration
}

func newSupervisor(
	starter Starter,
	waitForReadiness readinessWaiter,
	shutdownTimeout time.Duration,
) *supervisor {
	// The supervisor owns orchestration while Starter and readinessWaiter own I/O seams.
	return &supervisor{
		starter:          starter,
		waitForReadiness: waitForReadiness,
		shutdownTimeout:  shutdownTimeout,
	}
}

func (s *supervisor) Run(
	ctx context.Context,
	options LaunchOptions,
) error {
	// Start readiness checking concurrently so an early process exit or cancellation
	// cannot be hidden behind a health-check request.
	if ctx == nil {
		return fmt.Errorf("supervisor context is required")
	}
	if s.starter == nil {
		return fmt.Errorf("Collector starter is required")
	}
	if s.waitForReadiness == nil {
		return fmt.Errorf("Collector readiness waiter is required")
	}
	if s.shutdownTimeout <= 0 {
		return fmt.Errorf("Collector shutdown timeout must be positive")
	}

	lifecycle := newLifecycle()

	child, err := s.starter.Start(options)
	if err != nil {
		// No child exists after Start fails, so only the lifecycle state needs updating.
		_ = lifecycle.Transition(StateFailed)

		return fmt.Errorf("start Collector: %w", err)
	}

	readinessCtx, cancelReadiness := context.WithCancel(ctx)
	defer cancelReadiness()

	readinessDone := make(chan error, 1)

	go func() {
		readinessDone <- s.waitForReadiness(readinessCtx)
	}()

	select {
	case err := <-readinessDone:
		if err != nil {
			// A Collector that never becomes ready must be stopped before returning the
			// readiness error; otherwise bootstrap's caller would leak a process.
			if transitionErr := lifecycle.Transition(StateStopping); transitionErr != nil {
				return transitionErr
			}

			if stopErr := child.RequestStop(); stopErr != nil {
				_ = lifecycle.Transition(StateFailed)

				if killErr := s.forceKillAndWait(child); killErr != nil {
					return fmt.Errorf(
						"wait for Collector readiness: %w; request Collector shutdown: %v; %v",
						err,
						stopErr,
						killErr,
					)
				}

				return fmt.Errorf(
					"wait for Collector readiness: %w; request Collector shutdown: %v",
					err,
					stopErr,
				)
			}

			timer := time.NewTimer(s.shutdownTimeout)
			defer timer.Stop()

			select {
			case result := <-child.Wait():
				if result.Err != nil {
					_ = lifecycle.Transition(StateFailed)

					return fmt.Errorf(
						"wait for Collector readiness: %w; Collector exited with code %d: %v",
						err,
						result.Code,
						result.Err,
					)
				}

				if transitionErr := lifecycle.Transition(StateStopped); transitionErr != nil {
					return transitionErr
				}

				return fmt.Errorf("wait for Collector readiness: %w", err)

			case <-timer.C:
				// Escalate only after the graceful shutdown deadline has elapsed.
				if killErr := child.Kill(); killErr != nil {
					_ = lifecycle.Transition(StateFailed)

					return fmt.Errorf(
						"wait for Collector readiness: %w; force-kill Collector: %v",
						err,
						killErr,
					)
				}

				_ = lifecycle.Transition(StateFailed)

				return fmt.Errorf(
					"wait for Collector readiness: %w; Collector did not stop within %s and was force-killed",
					err,
					s.shutdownTimeout,
				)
			}
		}

		if transitionErr := lifecycle.Transition(StateReady); transitionErr != nil {
			return transitionErr
		}

	case result := <-child.Wait():
		// An exit before readiness is always a failed startup, even with exit code zero.
		_ = lifecycle.Transition(StateFailed)

		return unexpectedCollectorExit(result)

	case <-ctx.Done():
		// Cancellation before readiness still follows graceful shutdown to avoid orphaning the child.
		return s.shutdown(lifecycle, child)
	}

	select {
	case <-ctx.Done():
		return s.shutdown(lifecycle, child)

	case result := <-child.Wait():
		// Once ready, an unsolicited child exit is an unexpected supervisor failure.
		_ = lifecycle.Transition(StateFailed)

		return unexpectedCollectorExit(result)
	}
}

func (s *supervisor) shutdown(
	lifecycle *lifecycle,
	child Child,
) error {
	// Cancellation follows the same graceful-then-forced policy as readiness failure.
	if err := lifecycle.Transition(StateStopping); err != nil {
		return err
	}

	if err := child.RequestStop(); err != nil {
		_ = lifecycle.Transition(StateFailed)

		if killErr := s.forceKillAndWait(child); killErr != nil {
			return fmt.Errorf(
				"request Collector shutdown: %w; %v",
				err,
				killErr,
			)
		}

		return fmt.Errorf("request Collector shutdown: %w", err)
	}

	timer := time.NewTimer(s.shutdownTimeout)
	defer timer.Stop()

	select {
	case result := <-child.Wait():
		if result.Err != nil {
			_ = lifecycle.Transition(StateFailed)

			return fmt.Errorf(
				"Collector exited with code %d during shutdown: %w",
				result.Code,
				result.Err,
			)
		}

		if err := lifecycle.Transition(StateStopped); err != nil {
			return err
		}

		return nil

	case <-timer.C:
		// A second escalation is intentionally terminal: the caller must see that the
		// Collector did not honor its shutdown contract.
		if err := child.Kill(); err != nil {
			_ = lifecycle.Transition(StateFailed)

			return fmt.Errorf("force-kill Collector after shutdown timeout: %w", err)
		}

		_ = lifecycle.Transition(StateFailed)

		return fmt.Errorf(
			"Collector did not stop within %s and was force-killed",
			s.shutdownTimeout,
		)
	}
}

func (s *supervisor) forceKillAndWait(child Child) error {
	if err := child.Kill(); err != nil {
		return fmt.Errorf("force-kill Collector: %w", err)
	}

	timer := time.NewTimer(s.shutdownTimeout)
	defer timer.Stop()

	select {
	case result := <-child.Wait():
		if result.Err != nil {
			return fmt.Errorf(
				"Collector exited with code %d after force-kill: %w",
				result.Code,
				result.Err,
			)
		}

		return nil

	case <-timer.C:
		return fmt.Errorf(
			"Collector did not exit within %s after force-kill",
			s.shutdownTimeout,
		)
	}
}

func unexpectedCollectorExit(result ExitResult) error {
	// Preserve both the exit code and wait error because either explains a failed run.
	if result.Err != nil {
		return fmt.Errorf(
			"Collector exited unexpectedly with code %d: %w",
			result.Code,
			result.Err,
		)
	}

	return fmt.Errorf(
		"Collector exited unexpectedly with code %d",
		result.Code,
	)
}
