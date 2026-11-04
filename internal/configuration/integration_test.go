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
	}, Contribute: func(integration.IntegrationInstance) (integration.Contribution, error) {
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
