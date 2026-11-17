package application

import (
	"errors"
	"testing"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	definitionsapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

type memoryRepository struct{ writes int }

func (r *memoryRepository) Put(i desiredstatedomain.Instance, expected uint64) (desiredstatedomain.Instance, uint64, error) {
	r.writes++
	i.Revision = 1
	return i, 1, nil
}
func (*memoryRepository) Delete(string, uint64) (uint64, error) { return 0, nil }
func (*memoryRepository) Snapshot() (Snapshot, error)           { return Snapshot{}, nil }

func TestIntegrationServiceValidatesAndContributesBeforePersistence(t *testing.T) {
	d := definitionsdomain.Definition{ID: "redis", Version: "1.0.0", Validate: func(c map[string]any) error {
		if _, ok := c["endpoint"].(string); !ok {
			return errors.New("endpoint required")
		}
		return nil
	}, Contribute: func(map[string]any) (definitionsdomain.Contribution, error) {
		return definitionsdomain.NewContribution(nil), nil
	}}
	registry, err := definitionsapp.NewRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{}
	service := NewService(definitionResolverAdapter{resolver: definitionsapp.NewResolver(registry)}, repo)
	_, _, err = service.Put(desiredstatedomain.Instance{ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": 5}}, 0)
	if !errors.Is(err, desiredstatedomain.ErrInvalidInstance) || repo.writes != 0 {
		t.Fatalf("Put err=%v writes=%d", err, repo.writes)
	}
	_, _, err = service.Put(desiredstatedomain.Instance{ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": "localhost"}}, 0)
	if err != nil || repo.writes != 1 {
		t.Fatalf("valid Put err=%v writes=%d", err, repo.writes)
	}
}

type setEnabledRepository struct {
	snapshot Snapshot
	stored   desiredstatedomain.Instance
	expected uint64
	writes   int
}

func (r *setEnabledRepository) Put(value desiredstatedomain.Instance, expected uint64) (desiredstatedomain.Instance, uint64, error) {
	r.writes++
	r.stored = value
	r.expected = expected
	r.stored.Revision = expected + 1
	return r.stored, r.snapshot.Revision + 1, nil
}
func (*setEnabledRepository) Delete(string, uint64) (uint64, error) { return 0, nil }
func (r *setEnabledRepository) Snapshot() (Snapshot, error)         { return r.snapshot, nil }

func TestIntegrationServiceSetEnabledPreservesInstanceAndUsesExpectedRevision(t *testing.T) {
	definition := definitionsdomain.Definition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (definitionsdomain.Contribution, error) {
		return definitionsdomain.NewContribution(nil), nil
	}}
	registry, err := definitionsapp.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	existing := desiredstatedomain.Instance{
		ID: "redis-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true,
		Configuration: map[string]any{"endpoint": "redis.internal:6379", "namespace": "primary"}, Revision: 4,
	}
	repository := &setEnabledRepository{snapshot: Snapshot{Revision: 12, Instances: []desiredstatedomain.Instance{existing}}}
	service := NewService(definitionResolverAdapter{resolver: definitionsapp.NewResolver(registry)}, repository)

	stored, desiredRevision, err := service.SetEnabled("redis-a", false, 4)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Enabled || stored.ID != existing.ID || stored.DefinitionID != existing.DefinitionID || stored.DefinitionVersion != existing.DefinitionVersion {
		t.Fatalf("updated instance=%+v", stored)
	}
	if stored.Configuration["endpoint"] != "redis.internal:6379" || stored.Configuration["namespace"] != "primary" {
		t.Fatalf("configuration changed: %#v", stored.Configuration)
	}
	if repository.writes != 1 || repository.expected != 4 || desiredRevision != 13 {
		t.Fatalf("writes=%d expected=%d desiredRevision=%d", repository.writes, repository.expected, desiredRevision)
	}
}

func TestIntegrationServiceSetEnabledRejectsStaleRevisionWithoutWriting(t *testing.T) {
	registry, err := definitionsapp.NewRegistry(definitionsdomain.Definition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (definitionsdomain.Contribution, error) {
		return definitionsdomain.NewContribution(nil), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	repository := &setEnabledRepository{snapshot: Snapshot{Instances: []desiredstatedomain.Instance{{ID: "redis-a", Revision: 4}}}}
	service := NewService(definitionResolverAdapter{resolver: definitionsapp.NewResolver(registry)}, repository)
	_, _, err = service.SetEnabled("redis-a", false, 3)
	if !errors.Is(err, ErrRevisionConflict) || repository.writes != 0 {
		t.Fatalf("err=%v writes=%d", err, repository.writes)
	}
}

func TestIntegrationServiceSetEnabledReportsMissingInstance(t *testing.T) {
	registry, err := definitionsapp.NewRegistry(definitionsdomain.Definition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (definitionsdomain.Contribution, error) {
		return definitionsdomain.NewContribution(nil), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(definitionResolverAdapter{resolver: definitionsapp.NewResolver(registry)}, &setEnabledRepository{})
	_, _, err = service.SetEnabled("missing", true, 0)
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("err=%v, want ErrIntegrationNotFound", err)
	}
}

func TestIntegrationServicePutPreservingEnabledKeepsExistingEnabledValue(t *testing.T) {
	registry, err := definitionsapp.NewRegistry(definitionsdomain.Definition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (definitionsdomain.Contribution, error) {
		return definitionsdomain.NewContribution(nil), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	existing := desiredstatedomain.Instance{
		ID: "redis-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: false,
		Configuration: map[string]any{"namespace": "primary"}, Revision: 6,
	}
	repository := &setEnabledRepository{snapshot: Snapshot{Revision: 20, Instances: []desiredstatedomain.Instance{existing}}}
	service := NewService(definitionResolverAdapter{resolver: definitionsapp.NewResolver(registry)}, repository)

	stored, revision, err := service.PutPreservingEnabled(desiredstatedomain.Instance{
		ID: "redis-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true,
		Configuration: map[string]any{"namespace": "secondary"},
	}, 6)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Enabled || stored.Configuration["namespace"] != "secondary" || repository.writes != 1 || repository.expected != 6 || revision != 21 {
		t.Fatalf("stored=%+v writes=%d expected=%d revision=%d", stored, repository.writes, repository.expected, revision)
	}
}

type definitionResolverAdapter struct{ resolver definitionsapp.Resolver }

func (a definitionResolverAdapter) Resolve(instance desiredstatedomain.Instance) (desiredstatedomain.Instance, error) {
	if instance.Source == "" {
		instance.Source = desiredstatedomain.SourceLocal
	}
	if err := instance.Validate(); err != nil {
		return desiredstatedomain.Instance{}, err
	}
	resolved, err := a.resolver.Resolve(definitionsapp.ResolveInput{
		DefinitionID: instance.DefinitionID,
		Version:      instance.DefinitionVersion,
		Enabled:      instance.Enabled,
		Settings:     instance.Configuration,
	})
	if err != nil {
		if errors.Is(err, definitionsdomain.ErrUnknownDefinition) || errors.Is(err, definitionsdomain.ErrInvalidContribution) {
			return desiredstatedomain.Instance{}, err
		}
		return desiredstatedomain.Instance{}, desiredstatedomain.ErrInvalidInstance
	}
	instance.Configuration = resolved.Settings
	return instance, nil
}
