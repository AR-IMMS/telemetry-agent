package application

import (
	"fmt"
	"io"

	"github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
)

// InstallationIdentityRepository atomically stores a candidate only when no
// installation identity has been initialized and always returns the committed ID.
type InstallationIdentityRepository interface {
	// EnsureInstallationID returns the committed ID or an explicit state error.
	EnsureInstallationID(candidate domain.InstallationID) (domain.InstallationID, error)
}

// EnsureInstallationIdentity creates a candidate and returns the ID committed
// by the repository, which may belong to a concurrent initializer.
func EnsureInstallationIdentity(
	repository InstallationIdentityRepository,
	entropy io.Reader,
) (domain.InstallationID, error) {
	if repository == nil {
		return domain.InstallationID{}, fmt.Errorf("installation identity repository is required")
	}
	candidate, err := domain.NewInstallationID(entropy)
	if err != nil {
		return domain.InstallationID{}, err
	}
	committed, err := repository.EnsureInstallationID(candidate)
	if err != nil {
		return domain.InstallationID{}, fmt.Errorf("ensure installation identity: %w", err)
	}
	validated, err := domain.ParseInstallationID(committed.String())
	if err != nil {
		return domain.InstallationID{}, fmt.Errorf("validate committed installation identity: %w", err)
	}
	return validated, nil
}
