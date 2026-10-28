package identity

import (
	"fmt"
	"io"
)

// InstallationIdentityRepository atomically stores a candidate only when no
// installation identity has been initialized and always returns the committed ID.
type InstallationIdentityRepository interface {
	// EnsureInstallationID returns the committed ID or an explicit state error.
	EnsureInstallationID(candidate InstallationID) (InstallationID, error)
}

// EnsureInstallationIdentity creates a candidate and returns the ID committed
// by the repository, which may belong to a concurrent initializer.
func EnsureInstallationIdentity(
	repository InstallationIdentityRepository,
	entropy io.Reader,
) (InstallationID, error) {
	if repository == nil {
		return InstallationID{}, fmt.Errorf("installation identity repository is required")
	}
	candidate, err := NewInstallationID(entropy)
	if err != nil {
		return InstallationID{}, err
	}
	committed, err := repository.EnsureInstallationID(candidate)
	if err != nil {
		return InstallationID{}, fmt.Errorf("ensure installation identity: %w", err)
	}
	validated, err := ParseInstallationID(committed.String())
	if err != nil {
		return InstallationID{}, fmt.Errorf("validate committed installation identity: %w", err)
	}
	return validated, nil
}
