package configuration

import (
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

var (
	ErrRevisionConflict = errors.New("integration revision conflict")
	// ErrIntegrationNotFound identifies a desired Integration absent from the snapshot.
	ErrIntegrationNotFound = errors.New("Integration not found")
	// ErrTemplateOwnedInstance prevents local CRUD from mutating generated ownership records.
	ErrTemplateOwnedInstance = errors.New("Integration instance is owned by a Template")
)

// DesiredSnapshot is a consistent desired Integration set and its change marker.
type DesiredSnapshot struct {
	Revision  uint64
	Instances []integration.IntegrationInstance
}

// IntegrationRepository stores resolved desired instances using optimistic concurrency.
type IntegrationRepository interface {
	Put(integration.IntegrationInstance, uint64) (integration.IntegrationInstance, uint64, error)
	Delete(string, uint64) (uint64, error)
	Snapshot() (DesiredSnapshot, error)
}

// IntegrationService validates and resolves an instance before persisting it.
type IntegrationService struct {
	resolver   integration.Resolver
	repository IntegrationRepository
}

// NewIntegrationService wires the pure resolver to a desired-state repository.
func NewIntegrationService(resolver integration.Resolver, repository IntegrationRepository) IntegrationService {
	return IntegrationService{resolver: resolver, repository: repository}
}

// Put validates and resolves before applying an expected-revision mutation.
func (s IntegrationService) Put(instance integration.IntegrationInstance, expected uint64) (integration.IntegrationInstance, uint64, error) {
	if instance.Source == integration.SourceTemplate {
		return integration.IntegrationInstance{}, 0, ErrTemplateOwnedInstance
	}
	resolved, err := s.resolver.Resolve(instance)
	if err != nil {
		return integration.IntegrationInstance{}, 0, err
	}
	stored, revision, err := s.repository.Put(resolved.Instance, expected)
	if err != nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("store desired Integration: %w", err)
	}
	return stored, revision, nil
}

// SetEnabled changes only the desired lifecycle flag using the caller's expected instance revision.
// It preserves the stored definition and settings, then delegates validation, resolution and CAS to Put.
func (s IntegrationService) SetEnabled(id string, enabled bool, expected uint64) (integration.IntegrationInstance, uint64, error) {
	if s.repository == nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("Integration repository is required")
	}
	snapshot, err := s.repository.Snapshot()
	if err != nil {
		return integration.IntegrationInstance{}, 0, err
	}
	for _, instance := range snapshot.Instances {
		if instance.ID != id {
			continue
		}
		if instance.Revision != expected {
			return integration.IntegrationInstance{}, 0, ErrRevisionConflict
		}
		instance.Enabled = enabled
		return s.Put(instance, expected)
	}
	return integration.IntegrationInstance{}, 0, ErrIntegrationNotFound
}

// PutPreservingEnabled updates an existing Integration while retaining its current enabled state.
// The expected instance revision protects the read-modify-write from stale CLI input.
func (s IntegrationService) PutPreservingEnabled(instance integration.IntegrationInstance, expected uint64) (integration.IntegrationInstance, uint64, error) {
	if s.repository == nil {
		return integration.IntegrationInstance{}, 0, fmt.Errorf("Integration repository is required")
	}
	snapshot, err := s.repository.Snapshot()
	if err != nil {
		return integration.IntegrationInstance{}, 0, err
	}
	for _, existing := range snapshot.Instances {
		if existing.ID != instance.ID {
			continue
		}
		if existing.Revision != expected {
			return integration.IntegrationInstance{}, 0, ErrRevisionConflict
		}
		instance.Enabled = existing.Enabled
		return s.Put(instance, expected)
	}
	return integration.IntegrationInstance{}, 0, ErrIntegrationNotFound
}
