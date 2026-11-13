package agentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/credentialstore"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/opampsupervisorconfig"
	"github.com/ar-imms/telemetry-agent/internal/processowner"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

const managedSupervisorStartupTimeout = 30 * time.Second
const managedCredentialPollInterval = 5 * time.Second

var errSupervisorCredentialChanged = errors.New("Supervisor credential changed")

// runManagedSupervisor starts the sole Collector owner for one activated
// configuration snapshot and binds readiness to that exact snapshot.
func runManagedSupervisor(ctx context.Context, statePath string, managed agentstate.SupervisorContext, collector supervisor.Options) error {
	for {
		credentialReference, err := currentSupervisorCredentialReference(statePath)
		if err != nil {
			return err
		}
		err = runManagedSupervisorOnce(ctx, statePath, credentialReference, managed, collector)
		if !errors.Is(err, errSupervisorCredentialChanged) {
			return err
		}
	}
}

func runManagedSupervisorOnce(ctx context.Context, statePath, credentialReference string, managed agentstate.SupervisorContext, collector supervisor.Options) error {
	if !managed.Enabled || strings.TrimSpace(managed.BinaryPath) == "" || strings.TrimSpace(managed.OpAMPServerURL) == "" || strings.TrimSpace(managed.StateDirectory) == "" {
		return fmt.Errorf("enabled OpAMP Supervisor runtime context is incomplete")
	}
	credentials := credentialstore.New(statePath)
	tokenPath := ""
	if credentialReference != "" {
		var err error
		tokenPath, err = credentials.ProvisionSupervisorToken(credentialReference)
		if err != nil && !errors.Is(err, enrollment.ErrCredentialNotFound) {
			return fmt.Errorf("provision protected Supervisor bearer file: %w", err)
		}
	}
	if tokenPath == "" {
		if err := credentials.DeleteSupervisorToken(); err != nil {
			return err
		}
	}
	configBytes, err := opampsupervisorconfig.Render(opampsupervisorconfig.Config{
		ServerEndpoint: managed.OpAMPServerURL,
		ServerCAFile:   managed.ServerCAFile,
		TokenFile:      tokenPath,
		CollectorPath:  collector.BinaryPath,
		LocalConfig:    collector.ConfigPath,
		FallbackConfig: collector.ConfigPath,
		StateDirectory: managed.StateDirectory,
	})
	if err != nil {
		return fmt.Errorf("render protected OpAMP Supervisor configuration: %w", err)
	}
	configPath, err := credentials.WriteSupervisorConfig(configBytes)
	if err != nil {
		return err
	}
	if managed.ConfigurationPath != "" && !samePath(managed.ConfigurationPath, configPath) {
		return fmt.Errorf("Supervisor configuration path differs from the protected credential-store path")
	}
	stateDirectory, err := credentials.PrepareSupervisorStateDirectory()
	if err != nil {
		return err
	}
	if !samePath(managed.StateDirectory, stateDirectory) {
		return fmt.Errorf("Supervisor state directory differs from the protected credential-store path")
	}

	processContext, cancel := context.WithCancel(ctx)
	defer cancel()
	processDone := make(chan error, 1)
	go func() {
		processDone <- processowner.Run(processContext, processowner.Options{
			Executable:  managed.BinaryPath,
			Arguments:   []string{"--feature-gates=opampsupervisor.Extensions", "--config=" + configPath},
			Environment: []string{"OTEL_GATEWAY_ENDPOINT=" + collector.GatewayEndpoint},
		})
	}()
	credentialChanged := make(chan struct{}, 1)
	credentialWatchContext, cancelCredentialWatch := context.WithCancel(processContext)
	defer cancelCredentialWatch()
	go watchSupervisorCredential(credentialWatchContext, statePath, credentialReference, credentialChanged)
	readinessDone := make(chan error, 1)
	go func() {
		readinessDone <- waitForManagedCollectorReadiness(processContext, collector.HealthEndpoint, collector.ReadinessBody, managedSupervisorStartupTimeout)
	}()
	select {
	case err := <-readinessDone:
		if err != nil {
			cancel()
			processErr := <-processDone
			if processErr != nil && !errors.Is(processErr, context.Canceled) {
				return errors.Join(fmt.Errorf("wait for Collector readiness under OpAMP Supervisor: %w", err), processErr)
			}
			return fmt.Errorf("wait for Collector readiness under OpAMP Supervisor: %w", err)
		}
		if collector.OnReady != nil {
			if err := collector.OnReady(); err != nil {
				cancel()
				if processErr := <-processDone; processErr != nil {
					return errors.Join(fmt.Errorf("record Supervisor Collector readiness: %w", err), processErr)
				}
				return fmt.Errorf("record Supervisor Collector readiness: %w", err)
			}
		}
	case err := <-processDone:
		cancel()
		if err == nil {
			return fmt.Errorf("OpAMP Supervisor exited before Collector readiness")
		}
		return fmt.Errorf("OpAMP Supervisor exited before Collector readiness: %w", err)
	case <-credentialChanged:
		cancel()
		if err := <-processDone; err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("restart Supervisor after credential replacement: %w", err)
		}
		return errSupervisorCredentialChanged
	case <-ctx.Done():
		cancel()
		if err := <-processDone; err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop contained OpAMP Supervisor process tree: %w", err)
		}
		return nil
	}

	select {
	case err := <-processDone:
		if err == nil {
			return fmt.Errorf("OpAMP Supervisor exited unexpectedly")
		}
		return fmt.Errorf("OpAMP Supervisor exited unexpectedly: %w", err)
	case <-credentialChanged:
		cancel()
		if err := <-processDone; err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("restart Supervisor after credential replacement: %w", err)
		}
		return errSupervisorCredentialChanged
	case <-ctx.Done():
		cancel()
		if err := <-processDone; err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop contained OpAMP Supervisor process tree: %w", err)
		}
		return nil
	}
}

func currentSupervisorCredentialReference(statePath string) (string, error) {
	attempt, err := agentstate.NewEnrollmentStore(statePath).LoadAttempt()
	if errors.Is(err, enrollment.ErrNoAttempt) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load enrollment credential reference for Supervisor: %w", err)
	}
	if attempt.AccessCredentialRef != "" && attempt.Status == enrollment.AttemptRegistered {
		return attempt.AccessCredentialRef, nil
	}
	if attempt.PreviousAccessCredentialRef != "" {
		return attempt.PreviousAccessCredentialRef, nil
	}
	return "", nil
}

func watchSupervisorCredential(ctx context.Context, statePath, initialReference string, changed chan<- struct{}) {
	ticker := time.NewTicker(managedCredentialPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := currentSupervisorCredentialReference(statePath)
			if err == nil && current != initialReference {
				select {
				case changed <- struct{}{}:
				default:
				}
				return
			}
		}
	}
}

func waitForManagedCollectorReadiness(ctx context.Context, endpoint, body string, timeout time.Duration) error {
	if ctx == nil || strings.TrimSpace(endpoint) == "" || timeout <= 0 {
		return fmt.Errorf("Collector readiness context, endpoint, and timeout are required")
	}
	readinessContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(readinessContext, http.MethodGet, endpoint, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				content, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
				response.Body.Close()
				if readErr == nil && response.StatusCode >= 200 && response.StatusCode < 300 && (body == "" || strings.Contains(string(content), body)) {
					return nil
				}
			}
		}
		select {
		case <-readinessContext.Done():
			return readinessContext.Err()
		case <-ticker.C:
		}
	}
}

func samePath(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
