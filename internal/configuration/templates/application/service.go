package application

import (
	"errors"
	"fmt"
	"math"

	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	configurationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	domain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
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
	// ErrRevisionConflict preserves the shared optimistic concurrency error identity.
	ErrRevisionConflict = application.ErrRevisionConflict
)

// Snapshot is a consistent set of Templates and its write revision.
type Snapshot struct {
	Revision  uint64
	Templates []domain.Template
}

// Repository stores Templates with per-Template CAS and a set write marker.
type Repository interface {
	// Put stores a Template when its current revision equals expectedRevision.
	// A deleted Template ID must never be reused or re-enabled.
	Put(domain.Template, uint64) (domain.Template, uint64, error)
	// Snapshot returns all Templates, including tombstones, from one consistent read.
	Snapshot() (Snapshot, error)
}

// DefinitionResolver retrieves Monitoring-owned rules for an exact profile version.
type DefinitionResolver interface {
	ResolveDefinition(string, string) (definitions.Definition, error)
}

// Service validates local operator Templates before durable persistence.
// It does not discover resources, materialize Integration instances, or activate Collector.
type Service struct {
	definitions DefinitionResolver
	repository  Repository
}

// NewService creates the local Template application service.
func NewService(definitionResolver DefinitionResolver, repository Repository) Service {
	return Service{definitions: definitionResolver, repository: repository}
}

// Put creates or updates a local Template with optimistic revision checking.
// expectedRevision is zero for creation; a deleted ID remains permanently reserved.
func (s Service) Put(template domain.Template, expectedRevision uint64) (domain.Template, uint64, error) {
	if s.repository == nil {
		return domain.Template{}, 0, fmt.Errorf("Template repository is required")
	}
	if template.Owner != configurationdomain.TemplateOwnerLocal {
		return domain.Template{}, 0, ErrTemplateOwnerNotLocal
	}
	if expectedRevision >= math.MaxInt64 {
		return domain.Template{}, 0, fmt.Errorf("Template revision exceeds supported range")
	}
	template.Revision = expectedRevision + 1
	if err := template.Validate(); err != nil {
		return domain.Template{}, 0, err
	}
	definition, err := s.definitions.ResolveDefinition(template.DefinitionID, template.DefinitionVersion)
	if err != nil {
		return domain.Template{}, 0, err
	}
	if err := validateDefinition(template, definition); err != nil {
		return domain.Template{}, 0, err
	}
	stored, revision, err := s.repository.Put(template, expectedRevision)
	if err != nil {
		return domain.Template{}, 0, fmt.Errorf("store desired Integration Template: %w", err)
	}
	return stored, revision, nil
}

// Snapshot reads the current Template set through its owning repository.
func (s Service) Snapshot() (Snapshot, error) {
	if s.repository == nil {
		return Snapshot{}, fmt.Errorf("Template repository is required")
	}
	return s.repository.Snapshot()
}

// SetEnabled changes only the Template lifecycle state using its expected revision.
func (s Service) SetEnabled(id string, enabled bool, expectedRevision uint64) (domain.Template, uint64, error) {
	snapshot, err := s.Snapshot()
	if err != nil {
		return domain.Template{}, 0, err
	}
	for _, template := range snapshot.Templates {
		if template.ID != id {
			continue
		}
		if template.Revision != expectedRevision {
			return domain.Template{}, 0, ErrRevisionConflict
		}
		if template.Status == domain.StatusDeleted {
			return domain.Template{}, 0, ErrTemplateDeleted
		}
		if enabled {
			template.Status = domain.StatusEnabled
		} else {
			template.Status = domain.StatusDisabled
		}
		return s.Put(template, expectedRevision)
	}
	return domain.Template{}, 0, ErrTemplateNotFound
}

// Delete permanently tombstones a Template without deleting its ownership history.
func (s Service) Delete(id string, expectedRevision uint64) (domain.Template, uint64, error) {
	snapshot, err := s.Snapshot()
	if err != nil {
		return domain.Template{}, 0, err
	}
	for _, template := range snapshot.Templates {
		if template.ID != id {
			continue
		}
		if template.Revision != expectedRevision {
			return domain.Template{}, 0, ErrRevisionConflict
		}
		if template.Status == domain.StatusDeleted {
			return template, snapshot.Revision, nil
		}
		template.Status = domain.StatusDeleted
		return s.Put(template, expectedRevision)
	}
	return domain.Template{}, 0, ErrTemplateNotFound
}

func validateDefinition(template domain.Template, definition definitions.Definition) error {
	for key, value := range template.Settings {
		rule, ok := definition.TemplateSettingRules[key]
		if !ok {
			return fmt.Errorf("%w: Definition does not allow Template setting %q", domain.ErrInvalidTemplate, key)
		}
		if err := definitions.ValidateTemplateSettingValue(key, value, rule); err != nil {
			return err
		}
	}
	for _, binding := range template.Bindings {
		rule, ok := definition.TemplateSettingRules[binding.Target]
		if !ok || !rule.AllowResourceBinding {
			return fmt.Errorf("%w: Definition does not allow binding setting %q", domain.ErrInvalidTemplate, binding.Target)
		}
		if _, configured := template.Settings[binding.Target]; configured && !rule.BindingOverridesTemplate {
			return fmt.Errorf("%w: setting %q has both a Template value and resource binding", domain.ErrInvalidTemplate, binding.Target)
		}
	}
	bound := make(map[string]struct{}, len(template.Bindings))
	for _, binding := range template.Bindings {
		bound[binding.Target] = struct{}{}
	}
	for key, rule := range definition.TemplateSettingRules {
		if !rule.Required || rule.Default != nil {
			continue
		}
		if _, configured := template.Settings[key]; configured {
			continue
		}
		if _, configured := bound[key]; configured {
			continue
		}
		return fmt.Errorf("%w: required Template setting %q needs a value or binding", domain.ErrInvalidTemplate, key)
	}
	return nil
}
