package definitions

import (
	"errors"
	"testing"
)

func TestResolverUsesSettingsProjectionWithoutConfigurationModel(t *testing.T) {
	definition := Definition{
		ID:      "sample",
		Version: "1.0.0",
		Validate: func(settings map[string]any) error {
			if settings["endpoint"] != "127.0.0.1:6379" {
				return errors.New("endpoint was not projected")
			}
			return nil
		},
		Contribute: func(settings map[string]any) (Contribution, error) {
			intent, err := NewComponentIntent("otel.receiver.sample.metrics", settings)
			if err != nil {
				return Contribution{}, err
			}
			return NewContribution([]ComponentIntent{intent}), nil
		},
	}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(registry)

	resolved, err := resolver.Resolve(ResolveInput{
		DefinitionID: "sample",
		Version:      "1.0.0",
		Enabled:      true,
		Settings:     map[string]any{"endpoint": "127.0.0.1:6379"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resolved.Intents()); got != 1 {
		t.Fatalf("intent count = %d, want 1", got)
	}
}
