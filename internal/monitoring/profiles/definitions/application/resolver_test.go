package application

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestResolverUsesRegisteredDefinitionAndReturnsRuntimeIntent(t *testing.T) {
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
	resolved, err := NewResolver(registry).Resolve(ResolveInput{
		DefinitionID: "redis", Version: "1.0.0", Enabled: true,
		Settings: map[string]any{"endpoint": "127.0.0.1:6379"},
	})
	if err != nil {
		t.Fatal(err)
	}
	intents := resolved.Intents()
	if len(intents) != 1 || intents[0].Capability() != "otel.receiver.redis.metrics" {
		t.Fatalf("resolved intents=%v, want one Redis receiver intent", intents)
	}
	if got := string(intents[0].Configuration()); got != `{"endpoint":"127.0.0.1:6379"}` {
		t.Fatalf("intent configuration=%s", got)
	}
}
