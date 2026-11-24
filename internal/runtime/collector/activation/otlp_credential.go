package agentlifecycle

import (
	"errors"
	"fmt"
	"time"

	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	fleetenrollment "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	fleetdomain "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/domain"
)

// PrepareCollectorOTLPToken materializes only a committed Fleet OTLP bearer.
// The returned path may be passed to a child process; the bearer itself must
// remain in the protected file and must never enter the process environment.
func PrepareCollectorOTLPToken(statePath string, repository fleetenrollment.AttemptRepository) (string, error) {
	if repository == nil {
		return "", nil
	}
	attempt, err := repository.LoadAttempt()
	if errors.Is(err, fleetenrollment.ErrNoAttempt) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load Fleet OTLP credential reference: %w", err)
	}
	if attempt.Status != fleetdomain.AttemptRegistered || attempt.Result == nil || attempt.Result.OTLPCredentialID == "" {
		return "", nil
	}
	if attempt.OTLPCredentialRef == "" || attempt.Result.OTLPCredentialScope != "otlp.write" {
		return "", fmt.Errorf("registered Fleet OTLP credential metadata is incomplete")
	}
	if expiry := attempt.Result.OTLPCredentialExpiresAt; expiry != nil && !expiry.After(time.Now()) {
		return "", fmt.Errorf("registered Fleet OTLP credential is expired")
	}
	path, err := credentialstore.New(statePath).ProvisionOTLPToken(attempt.OTLPCredentialRef)
	if err != nil {
		return "", fmt.Errorf("provision protected Collector OTLP bearer file: %w", err)
	}
	return path, nil
}
