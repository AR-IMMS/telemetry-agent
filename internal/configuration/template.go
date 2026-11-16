package configuration

import (
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
)

var (
	// ErrTemplateOwnerNotLocal prevents local operations from impersonating Core authority.
	ErrTemplateOwnerNotLocal = templatesapp.ErrTemplateOwnerNotLocal
	// ErrTemplateNotFound identifies a Template absent from the durable snapshot.
	ErrTemplateNotFound = templatesapp.ErrTemplateNotFound
	// ErrTemplateDeleted identifies a permanently tombstoned Template.
	ErrTemplateDeleted = templatesapp.ErrTemplateDeleted
	// ErrTemplateTargetImmutable prevents a stable Template identity changing Integration type.
	ErrTemplateTargetImmutable = templatesapp.ErrTemplateTargetImmutable
)

// TemplateSnapshot preserves the previous name for Configuration's application snapshot.
type TemplateSnapshot = templatesapp.Snapshot

// TemplateRepository stores Templates with per-Template CAS and a set write marker.
type TemplateRepository = templatesapp.Repository

// TemplateService is a compatibility facade over Configuration's Template use case.
// It preserves the former Configuration API while delegating all decisions.
type TemplateService struct{ service templatesapp.Service }

// NewTemplateService creates the local Template application service.
func NewTemplateService(registry definitions.Registry, repository TemplateRepository) TemplateService {
	return TemplateService{service: templatesapp.NewService(registry, repository)}
}

// Put creates or updates a local Template with optimistic revision checking.
func (s TemplateService) Put(template integration.IntegrationTemplate, expectedRevision uint64) (integration.IntegrationTemplate, uint64, error) {
	return s.service.Put(template, expectedRevision)
}

// Snapshot reads the current Template set through its owning repository.
func (s TemplateService) Snapshot() (TemplateSnapshot, error) {
	return s.service.Snapshot()
}

// SetEnabled changes only the Template lifecycle state using its expected revision.
func (s TemplateService) SetEnabled(id string, enabled bool, expectedRevision uint64) (integration.IntegrationTemplate, uint64, error) {
	return s.service.SetEnabled(id, enabled, expectedRevision)
}

// Delete permanently tombstones a Template without deleting its ownership history.
func (s TemplateService) Delete(id string, expectedRevision uint64) (integration.IntegrationTemplate, uint64, error) {
	return s.service.Delete(id, expectedRevision)
}
