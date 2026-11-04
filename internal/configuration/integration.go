package configuration

import (
	"errors"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

var ErrRevisionConflict = errors.New("integration revision conflict")

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
