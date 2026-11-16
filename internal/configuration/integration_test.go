package configuration

import (
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

type memoryRepository struct{ writes int }

func (r *memoryRepository) Put(i integration.IntegrationInstance, expected uint64) (integration.IntegrationInstance, uint64, error) {
	r.writes++
	i.Revision = 1
	return i, 1, nil
}
func (*memoryRepository) Delete(string, uint64) (uint64, error) { return 0, nil }
func (*memoryRepository) Snapshot() (DesiredSnapshot, error)    { return DesiredSnapshot{}, nil }

func TestIntegrationServiceValidatesAndContributesBeforePersistence(t *testing.T) {
	d := integration.IntegrationDefinition{ID: "redis", Version: "1.0.0", Validate: func(c map[string]any) error {
		if _, ok := c["endpoint"].(string); !ok {
			return errors.New("endpoint required")
		}
		return nil
	}, Contribute: func(map[string]any) (integration.Contribution, error) {
		return integration.NewContribution(nil), nil
	}}
	registry, err := integration.NewRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{}
	service := NewIntegrationService(integration.NewResolver(registry), repo)
	_, _, err = service.Put(integration.IntegrationInstance{ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": 5}}, 0)
	if !errors.Is(err, integration.ErrInvalidInstance) || repo.writes != 0 {
		t.Fatalf("Put err=%v writes=%d", err, repo.writes)
	}
	_, _, err = service.Put(integration.IntegrationInstance{ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": "localhost"}}, 0)
	if err != nil || repo.writes != 1 {
		t.Fatalf("valid Put err=%v writes=%d", err, repo.writes)
	}
}

type setEnabledRepository struct {
	snapshot DesiredSnapshot
	stored   integration.IntegrationInstance
	expected uint64
	writes   int
}

func (r *setEnabledRepository) Put(value integration.IntegrationInstance, expected uint64) (integration.IntegrationInstance, uint64, error) {
	r.writes++
	r.stored = value
	r.expected = expected
	r.stored.Revision = expected + 1
	return r.stored, r.snapshot.Revision + 1, nil
}
func (*setEnabledRepository) Delete(string, uint64) (uint64, error) { return 0, nil }
func (r *setEnabledRepository) Snapshot() (DesiredSnapshot, error)  { return r.snapshot, nil }

func TestIntegrationServiceSetEnabledPreservesInstanceAndUsesExpectedRevision(t *testing.T) {
	definition := integration.IntegrationDefinition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (integration.Contribution, error) {
		return integration.NewContribution(nil), nil
	}}
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	existing := integration.IntegrationInstance{
		ID: "redis-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true,
		Configuration: map[string]any{"endpoint": "redis.internal:6379", "namespace": "primary"}, Revision: 4,
	}
	repository := &setEnabledRepository{snapshot: DesiredSnapshot{Revision: 12, Instances: []integration.IntegrationInstance{existing}}}
	service := NewIntegrationService(integration.NewResolver(registry), repository)

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
	registry, err := integration.NewRegistry(integration.IntegrationDefinition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (integration.Contribution, error) {
		return integration.NewContribution(nil), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	repository := &setEnabledRepository{snapshot: DesiredSnapshot{Instances: []integration.IntegrationInstance{{ID: "redis-a", Revision: 4}}}}
	service := NewIntegrationService(integration.NewResolver(registry), repository)
	_, _, err = service.SetEnabled("redis-a", false, 3)
	if !errors.Is(err, ErrRevisionConflict) || repository.writes != 0 {
		t.Fatalf("err=%v writes=%d", err, repository.writes)
	}
}

func TestIntegrationServiceSetEnabledReportsMissingInstance(t *testing.T) {
	registry, err := integration.NewRegistry(integration.IntegrationDefinition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (integration.Contribution, error) {
		return integration.NewContribution(nil), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	service := NewIntegrationService(integration.NewResolver(registry), &setEnabledRepository{})
	_, _, err = service.SetEnabled("missing", true, 0)
	if !errors.Is(err, ErrIntegrationNotFound) {
		t.Fatalf("err=%v, want ErrIntegrationNotFound", err)
	}
}

func TestIntegrationServicePutPreservingEnabledKeepsExistingEnabledValue(t *testing.T) {
	registry, err := integration.NewRegistry(integration.IntegrationDefinition{ID: "redis", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(map[string]any) (integration.Contribution, error) {
		return integration.NewContribution(nil), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	existing := integration.IntegrationInstance{
		ID: "redis-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: false,
		Configuration: map[string]any{"namespace": "primary"}, Revision: 6,
	}
	repository := &setEnabledRepository{snapshot: DesiredSnapshot{Revision: 20, Instances: []integration.IntegrationInstance{existing}}}
	service := NewIntegrationService(integration.NewResolver(registry), repository)

	stored, revision, err := service.PutPreservingEnabled(integration.IntegrationInstance{
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
