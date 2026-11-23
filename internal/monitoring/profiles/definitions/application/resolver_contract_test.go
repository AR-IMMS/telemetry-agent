package application

import (
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestResolverUsesSettingsProjectionWithoutConfigurationModel(t *testing.T) {
	definition := domain.Definition{
		ID:      "sample",
		Version: "1.0.0",
		Validate: func(settings map[string]any) error {
			if settings["endpoint"] != "127.0.0.1:6379" {
				return errors.New("endpoint was not projected")
			}
			return nil
		},
		Contribute: func(settings map[string]any) (domain.Contribution, error) {
			intent, err := domain.NewComponentIntent("otel.receiver.sample.metrics", settings)
			if err != nil {
				return domain.Contribution{}, err
			}
			return domain.NewContribution([]domain.ComponentIntent{intent}), nil
		},
	}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := NewResolver(registry).Resolve(ResolveInput{
		DefinitionID: "sample", Version: "1.0.0", Enabled: true,
		Settings: map[string]any{"endpoint": "127.0.0.1:6379"},
	})
	if err != nil {
		t.Fatal(err)
	}
	intents := resolved.Intents()
	if len(intents) != 1 || intents[0].Capability() != "otel.receiver.sample.metrics" {
		t.Fatalf("resolved intents=%v, want one sample receiver intent", intents)
	}
}
