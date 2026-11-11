package configuration

import (
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestTemplateServiceRejectsCoreOwnedOrUnknownDefinitionBeforePersistence(t *testing.T) {
	registry, err := integration.NewRegistry(templateTestDefinition())
	if err != nil {
		t.Fatal(err)
	}
	repository := &templateMemoryRepository{}
	service := NewTemplateService(registry, repository)

	tests := []struct {
		name   string
		change func(*integration.IntegrationTemplate)
	}{
		{"core owner", func(value *integration.IntegrationTemplate) { value.Owner = integration.TemplateOwnerCore }},
		{"unknown definition version", func(value *integration.IntegrationTemplate) { value.DefinitionVersion = "2.0.0" }},
		{"undeclared setting", func(value *integration.IntegrationTemplate) { value.Settings["password"] = "should-not-persist" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := templateTestValue()
			test.change(&candidate)
			if _, _, err := service.Put(candidate, 0); err == nil {
				t.Fatal("Put() accepted an invalid local template")
			}
			if repository.writes != 0 {
				t.Fatalf("repository writes = %d, want 0", repository.writes)
			}
		})
	}
}

func TestTemplateServiceStoresLocalEnabledTemplate(t *testing.T) {
	registry, err := integration.NewRegistry(templateTestDefinition())
	if err != nil {
		t.Fatal(err)
	}
	repository := &templateMemoryRepository{}
	service := NewTemplateService(registry, repository)
	stored, revision, err := service.Put(templateTestValue(), 0)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if stored.Owner != integration.TemplateOwnerLocal || stored.Revision != 1 || revision != 1 || repository.writes != 1 {
		t.Fatalf("stored=%+v revision=%d writes=%d", stored, revision, repository.writes)
	}
}

func TestTemplateServiceEnableDisableDeleteUsesCASAndPermanentTombstone(t *testing.T) {
	registry, err := integration.NewRegistry(templateTestDefinition())
	if err != nil {
		t.Fatal(err)
	}
	repository := &templateLifecycleMemoryRepository{templates: make(map[string]integration.IntegrationTemplate)}
	service := NewTemplateService(registry, repository)
	created, _, err := service.Put(templateTestValue(), 0)
	if err != nil {
		t.Fatal(err)
	}
	disabled, _, err := service.SetEnabled(created.ID, false, created.Revision)
	if err != nil || disabled.Status != integration.TemplateDisabled || disabled.Revision != 2 {
		t.Fatalf("disable=%+v err=%v", disabled, err)
	}
	if _, _, err := service.SetEnabled(created.ID, true, created.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale enable error=%v", err)
	}
	deleted, _, err := service.Delete(created.ID, disabled.Revision)
	if err != nil || deleted.Status != integration.TemplateDeleted || deleted.Revision != 3 {
		t.Fatalf("delete=%+v err=%v", deleted, err)
	}
	if _, _, err := service.SetEnabled(created.ID, true, deleted.Revision); !errors.Is(err, ErrTemplateDeleted) {
		t.Fatalf("deleted Template re-enable error=%v", err)
	}
	again, _, err := service.Delete(created.ID, deleted.Revision)
	if err != nil || again.Status != integration.TemplateDeleted || again.Revision != deleted.Revision {
		t.Fatalf("idempotent delete=%+v err=%v", again, err)
	}
}

type templateMemoryRepository struct {
	writes   int
	template integration.IntegrationTemplate
}

func (r *templateMemoryRepository) Put(value integration.IntegrationTemplate, expected uint64) (integration.IntegrationTemplate, uint64, error) {
	r.writes++
	if expected != 0 {
		return integration.IntegrationTemplate{}, 0, ErrRevisionConflict
	}
	value.Revision = 1
	r.template = value
	return value, 1, nil
}

func (*templateMemoryRepository) Snapshot() (TemplateSnapshot, error) {
	return TemplateSnapshot{}, errors.New("not implemented")
}

type templateLifecycleMemoryRepository struct {
	templates map[string]integration.IntegrationTemplate
	revision  uint64
}

func (r *templateLifecycleMemoryRepository) Put(value integration.IntegrationTemplate, expected uint64) (integration.IntegrationTemplate, uint64, error) {
	current, exists := r.templates[value.ID]
	if exists && current.Revision != expected || !exists && expected != 0 {
		return integration.IntegrationTemplate{}, 0, ErrRevisionConflict
	}
	if exists && current.Status == integration.TemplateDeleted && value.Status != integration.TemplateDeleted {
		return integration.IntegrationTemplate{}, 0, ErrTemplateDeleted
	}
	value.Revision = expected + 1
	r.revision++
	r.templates[value.ID] = value
	return value, r.revision, nil
}

func (r *templateLifecycleMemoryRepository) Snapshot() (TemplateSnapshot, error) {
	values := make([]integration.IntegrationTemplate, 0, len(r.templates))
	for _, value := range r.templates {
		values = append(values, value)
	}
	return TemplateSnapshot{Revision: r.revision, Templates: values}, nil
}

func templateTestDefinition() integration.IntegrationDefinition {
	return integration.IntegrationDefinition{
		ID: "redis", Version: "1.0.0",
		Validate: func(map[string]any) error { return nil },
		Contribute: func(integration.IntegrationInstance) (integration.Contribution, error) {
			return integration.NewContribution(nil), nil
		},
		TemplateSettingRules: map[string]integration.TemplateSettingRule{
			"endpoint": {AllowResourceBinding: true},
		},
	}
}

func templateTestValue() integration.IntegrationTemplate {
	return integration.IntegrationTemplate{
		ID: "redis-default", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Owner: integration.TemplateOwnerLocal, Revision: 1, Status: integration.TemplateEnabled,
		Settings: map[string]any{},
		Bindings: []integration.TemplateSettingBinding{{Target: "endpoint", Source: integration.TemplateBindingSource{
			Kind: integration.TemplateSourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}
