package application

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestResolverReturnsIndependentSettingsAndContributionCopies(t *testing.T) {
	definition := domain.Definition{
		ID: "redis", Version: "1.0.0",
		Validate: func(settings map[string]any) error { return nil },
		Contribute: func(settings map[string]any) (domain.Contribution, error) {
			intent, err := domain.NewComponentIntent("otel.receiver.redis.metrics", map[string]any{"endpoint": settings["endpoint"]})
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
	input := ResolveInput{DefinitionID: "redis", Version: "1.0.0", Enabled: true, Settings: map[string]any{"endpoint": "127.0.0.1:6379"}}
	resolved, err := NewResolver(registry).Resolve(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Settings["endpoint"] = "127.0.0.1:9999"
	if got := resolved.Settings["endpoint"]; got != "127.0.0.1:6379" {
		t.Fatalf("resolved endpoint=%v, want original projection", got)
	}
	if got := string(resolved.Intents()[0].Configuration()); got != `{"endpoint":"127.0.0.1:6379"}` {
		t.Fatalf("intent configuration=%s", got)
	}
}
