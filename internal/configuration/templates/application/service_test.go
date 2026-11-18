package application

import (
	"errors"
	"testing"

	domain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	definitions "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestServiceCreatesUpdatesAndTombstonesTemplateWithCAS(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(testDefinitionResolver{}, repository)

	created, setRevision, err := service.Put(templateFixture(), 0)
	if err != nil || created.Revision != 1 || setRevision != 1 {
		t.Fatalf("Put() = (%+v, %d, %v), want revision 1 and set revision 1", created, setRevision, err)
	}

	disabled, setRevision, err := service.SetEnabled(created.ID, false, created.Revision)
	if err != nil || disabled.Status != domain.StatusDisabled || disabled.Revision != 2 || setRevision != 2 {
		t.Fatalf("SetEnabled() = (%+v, %d, %v), want disabled revision 2", disabled, setRevision, err)
	}

	deleted, setRevision, err := service.Delete(disabled.ID, disabled.Revision)
	if err != nil || deleted.Status != domain.StatusDeleted || deleted.Revision != 3 || setRevision != 3 {
		t.Fatalf("Delete() = (%+v, %d, %v), want tombstone revision 3", deleted, setRevision, err)
	}
	if repository.writes != 3 {
		t.Fatalf("repository writes = %d, want 3", repository.writes)
	}
}

func TestServiceRejectsUnknownSettingBeforeRepositoryWrite(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(testDefinitionResolver{}, repository)
	template := templateFixture()
	template.Settings = map[string]any{"unapproved": true}

	if _, _, err := service.Put(template, 0); err == nil {
		t.Fatal("Put() accepted a setting absent from the resolved profile definition")
	}
	if repository.writes != 0 {
		t.Fatalf("repository writes = %d, want no write after validation failure", repository.writes)
	}
}

type testDefinitionResolver struct{}

func (testDefinitionResolver) ResolveDefinition(id, version string) (definitions.Definition, error) {
	if id != "redis" || version != "1.0.0" {
		return definitions.Definition{}, errors.New("unexpected profile identity")
	}
	return definitions.Definition{
		ID: id, Version: version,
		Validate:   func(map[string]any) error { return nil },
		Contribute: func(map[string]any) (definitions.Contribution, error) { return definitions.NewContribution(nil), nil },
		TemplateSettingRules: map[string]definitions.TemplateSettingRule{
			"endpoint": {Type: "string", Description: "Redis endpoint", Required: true, AllowResourceBinding: true},
		},
	}, nil
}

type memoryRepository struct {
	templates map[string]domain.Template
	setRev    uint64
	writes    int
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{templates: make(map[string]domain.Template)}
}

func (r *memoryRepository) Put(template domain.Template, expected uint64) (domain.Template, uint64, error) {
	current, exists := r.templates[template.ID]
	if exists && current.Revision != expected || !exists && expected != 0 {
		return domain.Template{}, r.setRev, ErrRevisionConflict
	}
	template.Revision = expected + 1
	r.templates[template.ID] = template
	r.setRev++
	r.writes++
	return template, r.setRev, nil
}

func (r *memoryRepository) Snapshot() (Snapshot, error) {
	templates := make([]domain.Template, 0, len(r.templates))
	for _, template := range r.templates {
		templates = append(templates, template)
	}
	return Snapshot{Revision: r.setRev, Templates: templates}, nil
}

func templateFixture() domain.Template {
	return domain.Template{
		ID: "redis-default", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Owner: "local", Status: domain.StatusEnabled,
		Bindings: []domain.SettingBinding{{Target: "endpoint", Source: domain.BindingSource{
			Kind: domain.SourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}
