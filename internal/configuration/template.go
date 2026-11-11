package configuration

import (
	"errors"
	"fmt"
	"math"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

var (
	// ErrTemplateOwnerNotLocal prevents local operations from impersonating Core authority.
	ErrTemplateOwnerNotLocal = errors.New("only local Integration Templates are writable")
	// ErrTemplateNotFound identifies a Template absent from the durable snapshot.
	ErrTemplateNotFound = errors.New("Integration Template not found")
	// ErrTemplateDeleted identifies a permanently tombstoned Template.
	ErrTemplateDeleted = errors.New("Integration Template is deleted")
	// ErrTemplateTargetImmutable prevents a stable Template identity changing Integration type.
	ErrTemplateTargetImmutable = errors.New("Integration Template target is immutable")
)

// TemplateSnapshot is a consistent set of Templates and its write revision.
type TemplateSnapshot struct {
	Revision  uint64
	Templates []integration.IntegrationTemplate
}

// TemplateRepository stores Templates with per-Template CAS and a set write marker.
type TemplateRepository interface {
	// Put stores a Template when its current revision equals expectedRevision.
	// A deleted Template ID must never be reused or re-enabled.
	Put(integration.IntegrationTemplate, uint64) (integration.IntegrationTemplate, uint64, error)
	// Snapshot returns all Templates, including tombstones, from one consistent read.
	Snapshot() (TemplateSnapshot, error)
}

// TemplateService validates local operator Templates before durable persistence.
// It does not discover resources, materialize Integration instances, or activate Collector.
type TemplateService struct {
	registry   integration.Registry
	repository TemplateRepository
}

// NewTemplateService creates the local Template application service.
func NewTemplateService(registry integration.Registry, repository TemplateRepository) TemplateService {
	return TemplateService{registry: registry, repository: repository}
}

// Put creates or updates a local Template with optimistic revision checking.
// expectedRevision is zero for creation; a deleted ID remains permanently reserved.
func (s TemplateService) Put(template integration.IntegrationTemplate, expectedRevision uint64) (integration.IntegrationTemplate, uint64, error) {
	if s.repository == nil {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("Template repository is required")
	}
	if template.Owner != integration.TemplateOwnerLocal {
		return integration.IntegrationTemplate{}, 0, ErrTemplateOwnerNotLocal
	}
	if expectedRevision >= math.MaxInt64 {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("Template revision exceeds supported range")
	}
	template.Revision = expectedRevision + 1
	if err := integration.ValidateIntegrationTemplate(template); err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	definition, err := s.registry.ResolveDefinition(template.DefinitionID, template.DefinitionVersion)
	if err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	if err := validateTemplateDefinition(template, definition); err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	stored, revision, err := s.repository.Put(template, expectedRevision)
	if err != nil {
		return integration.IntegrationTemplate{}, 0, fmt.Errorf("store desired Integration Template: %w", err)
	}
	return stored, revision, nil
}

// Snapshot reads the current Template set through its owning repository.
func (s TemplateService) Snapshot() (TemplateSnapshot, error) {
	if s.repository == nil {
		return TemplateSnapshot{}, fmt.Errorf("Template repository is required")
	}
	return s.repository.Snapshot()
}

// SetEnabled changes only the Template lifecycle state using its expected revision.
func (s TemplateService) SetEnabled(id string, enabled bool, expectedRevision uint64) (integration.IntegrationTemplate, uint64, error) {
	snapshot, err := s.Snapshot()
	if err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	for _, template := range snapshot.Templates {
		if template.ID != id {
			continue
		}
		if template.Revision != expectedRevision {
			return integration.IntegrationTemplate{}, 0, ErrRevisionConflict
		}
		if template.Status == integration.TemplateDeleted {
			return integration.IntegrationTemplate{}, 0, ErrTemplateDeleted
		}
		if enabled {
			template.Status = integration.TemplateEnabled
		} else {
			template.Status = integration.TemplateDisabled
		}
		return s.Put(template, expectedRevision)
	}
	return integration.IntegrationTemplate{}, 0, ErrTemplateNotFound
}

// Delete permanently tombstones a Template without deleting its ownership history.
func (s TemplateService) Delete(id string, expectedRevision uint64) (integration.IntegrationTemplate, uint64, error) {
	snapshot, err := s.Snapshot()
	if err != nil {
		return integration.IntegrationTemplate{}, 0, err
	}
	for _, template := range snapshot.Templates {
		if template.ID != id {
			continue
		}
		if template.Revision != expectedRevision {
			return integration.IntegrationTemplate{}, 0, ErrRevisionConflict
		}
		if template.Status == integration.TemplateDeleted {
			return template, snapshot.Revision, nil
		}
		template.Status = integration.TemplateDeleted
		return s.Put(template, expectedRevision)
	}
	return integration.IntegrationTemplate{}, 0, ErrTemplateNotFound
}

func validateTemplateDefinition(template integration.IntegrationTemplate, definition integration.IntegrationDefinition) error {
	for key := range template.Settings {
		if _, ok := definition.TemplateSettingRules[key]; !ok {
			return fmt.Errorf("%w: Definition does not allow Template setting %q", integration.ErrInvalidTemplate, key)
		}
	}
	for _, binding := range template.Bindings {
		rule, ok := definition.TemplateSettingRules[binding.Target]
		if !ok || !rule.AllowResourceBinding {
			return fmt.Errorf("%w: Definition does not allow binding setting %q", integration.ErrInvalidTemplate, binding.Target)
		}
	}
	return nil
}
