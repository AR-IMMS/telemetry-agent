package credentialstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/enrollment"
)

// Store persists credentials outside SQLite in a service-scoped protected
// directory. Platform files enforce OS-specific encryption or permissions.
type Store struct {
	directory string
}

// New creates a Store beside the operational database, in a dedicated
// credentials directory shared by the interactive enrollment command and Agent.
func New(statePath string) Store {
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return Store{}
	}
	return Store{directory: filepath.Join(filepath.Dir(statePath), "credentials")}
}

// Put atomically stores a secret under a stable opaque reference.
func (s Store) Put(reference, value string) error {
	if err := s.validate(reference, value); err != nil {
		return err
	}
	if err := secureDirectory(s.directory); err != nil {
		return fmt.Errorf("secure credential directory: %w", err)
	}
	plain := []byte(value)
	defer clear(plain)
	sealed, err := protectSecret(plain)
	if err != nil {
		return fmt.Errorf("protect credential: %w", err)
	}
	defer clear(sealed)
	if err := writeAtomicFile(s.directory, s.path(reference), sealed); err != nil {
		return fmt.Errorf("persist protected credential: %w", err)
	}
	return nil
}

// Get reads and opens a protected secret. It rejects files with unsafe type,
// ownership, or permission metadata before returning their contents.
func (s Store) Get(reference string) (string, error) {
	if err := s.validate(reference, "present"); err != nil {
		return "", err
	}
	if err := secureDirectory(s.directory); err != nil {
		return "", fmt.Errorf("secure credential directory: %w", err)
	}
	content, err := readSecretFile(s.directory, s.path(reference))
	if err != nil {
		if os.IsNotExist(err) {
			return "", enrollment.ErrCredentialNotFound
		}
		return "", fmt.Errorf("read protected credential: %w", err)
	}
	defer clear(content)
	plain, err := unprotectSecret(content)
	if err != nil {
		return "", fmt.Errorf("unprotect credential")
	}
	defer clear(plain)
	if len(plain) == 0 {
		return "", fmt.Errorf("protected credential is empty")
	}
	return string(plain), nil
}

// Delete removes a credential idempotently and synchronizes the directory entry.
func (s Store) Delete(reference string) error {
	if err := s.validate(reference, "present"); err != nil {
		return err
	}
	if err := secureDirectory(s.directory); err != nil {
		return fmt.Errorf("secure credential directory: %w", err)
	}
	if err := os.Remove(s.path(reference)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete protected credential: %w", err)
	}
	if err := syncDirectory(s.directory); err != nil {
		return fmt.Errorf("sync credential directory after deletion: %w", err)
	}
	return nil
}

func (s Store) path(reference string) string {
	digest := sha256.Sum256([]byte(reference))
	return filepath.Join(s.directory, hex.EncodeToString(digest[:]))
}

func (s Store) validate(reference, value string) error {
	if strings.TrimSpace(s.directory) == "" {
		return fmt.Errorf("credential directory is required")
	}
	if strings.TrimSpace(reference) == "" || strings.TrimSpace(value) == "" {
		return fmt.Errorf("credential reference and value are required")
	}
	return nil
}
