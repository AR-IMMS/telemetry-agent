package configuration

import (
	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

var (
	// ErrRevisionConflict reports an optimistic concurrency mismatch.
	ErrRevisionConflict = application.ErrRevisionConflict
	// ErrIntegrationNotFound identifies a desired Integration absent from the snapshot.
	ErrIntegrationNotFound = application.ErrInstanceNotFound
	// ErrTemplateOwnedInstance prevents local CRUD from mutating generated ownership records.
	ErrTemplateOwnedInstance = application.ErrTemplateOwnedInstance
)

// DesiredSnapshot preserves the existing name for the application-owned snapshot.
type DesiredSnapshot = application.Snapshot

// IntegrationRepository preserves the existing repository name for desired state.
type IntegrationRepository = application.Repository

// IntegrationService preserves the prior API while delegating to its application capability.
type IntegrationService struct{ service application.Service }

// NewIntegrationService wires the Monitoring resolver and desired-state repository through ports.
func NewIntegrationService(resolver integration.Resolver, repository IntegrationRepository) IntegrationService {
	return IntegrationService{service: application.NewService(resolverAdapter{resolver: resolver}, repository)}
}

// Put validates and resolves before applying an expected-revision mutation.
func (s IntegrationService) Put(instance integration.IntegrationInstance, expected uint64) (integration.IntegrationInstance, uint64, error) {
	return s.service.Put(instance, expected)
}

// SetEnabled changes the desired lifecycle flag using the expected instance revision.
func (s IntegrationService) SetEnabled(id string, enabled bool, expected uint64) (integration.IntegrationInstance, uint64, error) {
	return s.service.SetEnabled(id, enabled, expected)
}

// PutPreservingEnabled updates an instance while retaining its current enabled state.
func (s IntegrationService) PutPreservingEnabled(instance integration.IntegrationInstance, expected uint64) (integration.IntegrationInstance, uint64, error) {
	return s.service.PutPreservingEnabled(instance, expected)
}

type resolverAdapter struct{ resolver integration.Resolver }

func (a resolverAdapter) Resolve(instance integration.IntegrationInstance) (integration.IntegrationInstance, error) {
	resolved, err := a.resolver.Resolve(instance)
	if err != nil {
		return integration.IntegrationInstance{}, err
	}
	return resolved.Instance, nil
}
