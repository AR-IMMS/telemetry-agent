package agentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	fleetenrollment "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	fleetdomain "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/supervisor"
	"github.com/ar-imms/telemetry-agent/internal/runtime/opamp"
	opampdomain "github.com/ar-imms/telemetry-agent/internal/runtime/opamp/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/process"
)

const managedSupervisorStartupTimeout = 30 * time.Second
const managedCredentialPollInterval = 5 * time.Second

var errSupervisorCredentialChanged = errors.New("Supervisor credential changed")

// runManagedSupervisor starts the sole Collector owner for one activated
// configuration snapshot and binds readiness to that exact snapshot.
func runManagedSupervisor(ctx context.Context, statePath string, managed opampdomain.SupervisorContext, collector supervisor.Options, attemptRepository enrollment.AttemptRepository, fleetRepository fleetenrollment.AttemptRepository) error {
	if attemptRepository == nil {
		return fmt.Errorf("Supervisor enrollment repository is required")
	}
	for {
		credentialReference, err := currentSupervisorCredentialReference(attemptRepository, fleetRepository, managed)
		if err != nil {
			return err
		}
		err = runManagedSupervisorOnce(ctx, statePath, credentialReference, managed, collector, attemptRepository, fleetRepository)
		if !errors.Is(err, errSupervisorCredentialChanged) {
			return err
		}
	}
}

func runManagedSupervisorOnce(ctx context.Context, statePath, credentialReference string, managed opampdomain.SupervisorContext, collector supervisor.Options, attemptRepository enrollment.AttemptRepository, fleetRepository fleetenrollment.AttemptRepository) error {
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
	go watchSupervisorCredential(credentialWatchContext, attemptRepository, fleetRepository, managed, credentialReference, credentialChanged)
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

func currentSupervisorCredentialReference(repository enrollment.AttemptRepository, fleetRepository fleetenrollment.AttemptRepository, supervisor opampdomain.SupervisorContext) (string, error) {
	endpoint := strings.TrimSpace(supervisor.OpAMPServerURL)
	switch supervisor.CredentialSource {
	case opampdomain.CredentialSourceFleet:
		return fleetSupervisorCredentialReference(fleetRepository, endpoint)
	case opampdomain.CredentialSourceCore:
		if fleetRepository != nil {
			attempt, err := fleetRepository.LoadAttempt()
			if err == nil && attempt.Result != nil && strings.TrimSpace(attempt.Result.OpAMPEndpoint) == endpoint {
				return "", fmt.Errorf("core credential source conflicts with registered Fleet endpoint")
			}
			if err != nil && !errors.Is(err, fleetenrollment.ErrNoAttempt) {
				return "", fmt.Errorf("load Fleet endpoint binding for Supervisor: %w", err)
			}
		}
		return coreSupervisorCredentialReference(repository)
	case "":
		// Older local Supervisor state has no issuer field. A registered Fleet
		// attempt is still usable only when its endpoint matches exactly. A
		// remote endpoint without matching Fleet state is ambiguous and must not
		// receive the legacy Core credential.
		if fleetRepository != nil {
			attempt, err := fleetRepository.LoadAttempt()
			if err == nil {
				if attempt.Status != fleetdomain.AttemptRegistered || attempt.Result == nil || attempt.CredentialRef == "" ||
					strings.TrimSpace(attempt.Result.OpAMPEndpoint) != endpoint {
					return "", fmt.Errorf("legacy Supervisor endpoint does not match registered Fleet enrollment state")
				}
				return attempt.CredentialRef, nil
			}
			if !errors.Is(err, fleetenrollment.ErrNoAttempt) {
				return "", fmt.Errorf("load Fleet credential reference for Supervisor: %w", err)
			}
		}
		if !isLoopbackSupervisorEndpoint(endpoint) {
			return "", fmt.Errorf("refusing Core Supervisor credential for remote endpoint without an explicit credential source")
		}
		return coreSupervisorCredentialReference(repository)
	default:
		return "", fmt.Errorf("unsupported Supervisor credential source %q", supervisor.CredentialSource)
	}
}

func fleetSupervisorCredentialReference(repository fleetenrollment.AttemptRepository, endpoint string) (string, error) {
	if repository == nil {
		return "", fmt.Errorf("Fleet Supervisor endpoint has no Fleet enrollment repository")
	}
	attempt, err := repository.LoadAttempt()
	if err != nil {
		return "", fmt.Errorf("load Fleet credential reference for Supervisor: %w", err)
	}
	if attempt.Status != fleetdomain.AttemptRegistered || attempt.Result == nil || attempt.CredentialRef == "" {
		return "", fmt.Errorf("Fleet Supervisor endpoint has no registered Fleet credential")
	}
	if strings.TrimSpace(attempt.Result.OpAMPEndpoint) != endpoint {
		return "", fmt.Errorf("Fleet Supervisor endpoint does not match registered Fleet enrollment state")
	}
	return attempt.CredentialRef, nil
}

func coreSupervisorCredentialReference(repository enrollment.AttemptRepository) (string, error) {
	attempt, err := repository.LoadAttempt()
	if errors.Is(err, enrollment.ErrNoAttempt) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load Core enrollment credential reference for Supervisor: %w", err)
	}
	if attempt.AccessCredentialRef != "" && attempt.Status == enrollment.AttemptRegistered {
		return attempt.AccessCredentialRef, nil
	}
	if attempt.PreviousAccessCredentialRef != "" {
		return attempt.PreviousAccessCredentialRef, nil
	}
	return "", nil
}

func isLoopbackSupervisorEndpoint(endpoint string) bool {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return false
	}
	host := strings.TrimSpace(parsed.Hostname())
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func watchSupervisorCredential(ctx context.Context, repository enrollment.AttemptRepository, fleetRepository fleetenrollment.AttemptRepository, supervisor opampdomain.SupervisorContext, initialReference string, changed chan<- struct{}) {
	ticker := time.NewTicker(managedCredentialPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := currentSupervisorCredentialReference(repository, fleetRepository, supervisor)
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
