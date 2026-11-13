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

const supervisorTokenFileName = "opamp-supervisor.token"
const supervisorConfigFileName = "opamp-supervisor.yaml"

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
	plain, err := s.getBytes(reference)
	if err != nil {
		return "", err
	}
	defer clear(plain)
	return string(plain), nil
}

// ProvisionSupervisorToken writes the existing enrollment bearer to the fixed
// token-file path protected by the same OS ACLs or permissions as this store.
func (s Store) ProvisionSupervisorToken(reference string) (string, error) {
	plain, err := s.getBytes(reference)
	if err != nil {
		return "", err
	}
	defer clear(plain)
	path := filepath.Join(s.directory, supervisorTokenFileName)
	if err := writeAtomicFile(s.directory, path, plain); err != nil {
		return "", fmt.Errorf("provision protected Supervisor token file: %w", err)
	}
	return path, nil
}

// DeleteSupervisorToken removes the materialized bearer token file idempotently.
func (s Store) DeleteSupervisorToken() error {
	if strings.TrimSpace(s.directory) == "" {
		return fmt.Errorf("credential directory is required")
	}
	if err := secureDirectory(s.directory); err != nil {
		return fmt.Errorf("secure credential directory: %w", err)
	}
	if err := os.Remove(filepath.Join(s.directory, supervisorTokenFileName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete protected Supervisor token file: %w", err)
	}
	if err := syncDirectory(s.directory); err != nil {
		return fmt.Errorf("sync credential directory after token removal: %w", err)
	}
	return nil
}

// WriteSupervisorConfig atomically writes the non-secret runtime config beside
// the protected token file so the Supervisor can read both under one ACL.
func (s Store) WriteSupervisorConfig(content []byte) (string, error) {
	if strings.TrimSpace(s.directory) == "" || len(content) == 0 || len(content) > 256<<10 {
		return "", fmt.Errorf("Supervisor configuration directory and bounded content are required")
	}
	if err := secureDirectory(s.directory); err != nil {
		return "", fmt.Errorf("secure credential directory: %w", err)
	}
	path := filepath.Join(s.directory, supervisorConfigFileName)
	if err := writeAtomicFile(s.directory, path, content); err != nil {
		return "", fmt.Errorf("write protected Supervisor configuration: %w", err)
	}
	return path, nil
}

// DeleteSupervisorConfig removes the generated local configuration idempotently.
func (s Store) DeleteSupervisorConfig() error {
	if strings.TrimSpace(s.directory) == "" {
		return fmt.Errorf("credential directory is required")
	}
	if err := secureDirectory(s.directory); err != nil {
		return fmt.Errorf("secure credential directory: %w", err)
	}
	if err := os.Remove(filepath.Join(s.directory, supervisorConfigFileName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete protected Supervisor configuration: %w", err)
	}
	if err := syncDirectory(s.directory); err != nil {
		return fmt.Errorf("sync credential directory after Supervisor configuration removal: %w", err)
	}
	return nil
}

// PrepareSupervisorStateDirectory creates persistent Supervisor storage under
// the same protected credential directory and applies platform protections.
func (s Store) PrepareSupervisorStateDirectory() (string, error) {
	if strings.TrimSpace(s.directory) == "" {
		return "", fmt.Errorf("credential directory is required")
	}
	path := filepath.Join(s.directory, "opamp-supervisor-state")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("create Supervisor state directory: %w", err)
	}
	if err := secureDirectory(path); err != nil {
		return "", fmt.Errorf("secure Supervisor state directory: %w", err)
	}
	return path, nil
}

func (s Store) getBytes(reference string) ([]byte, error) {
	if err := s.validate(reference, "present"); err != nil {
		return nil, err
	}
	if err := secureDirectory(s.directory); err != nil {
		return nil, fmt.Errorf("secure credential directory: %w", err)
	}
	content, err := readSecretFile(s.directory, s.path(reference))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, enrollment.ErrCredentialNotFound
		}
		return nil, fmt.Errorf("read protected credential: %w", err)
	}
	defer clear(content)
	plain, err := unprotectSecret(content)
	if err != nil {
		return nil, fmt.Errorf("unprotect credential")
	}
	if len(plain) == 0 {
		clear(plain)
		return nil, fmt.Errorf("protected credential is empty")
	}
	return plain, nil
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
