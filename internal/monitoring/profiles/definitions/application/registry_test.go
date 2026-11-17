package application

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitiondomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestRegistryReturnsIndependentDefinitionMetadata(t *testing.T) {
	definition := definitiondomain.Definition{
		ID: "redis", Version: "1.0.0",
		Validate: func(map[string]any) error { return nil },
		Contribute: func(map[string]any) (definitiondomain.Contribution, error) {
			return definitiondomain.NewContribution(nil), nil
		},
		TemplateSettingRules: map[string]definitiondomain.TemplateSettingRule{"labels": {Type: "object", Description: "Non-secret labels", Default: map[string]any{"team": "agent"}}},
		Matching: &domain.MatchingDefinition{
			Selector:             &domain.MatchSelector{ExactAttributes: map[string]string{"container.image": "redis"}},
			RequiredCapabilities: []string{"redisreceiver"},
		},
	}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	first := registry.Definitions()
	first[0].TemplateSettingRules["labels"].Default.(map[string]any)["team"] = "mutated"
	first[0].Matching.RequiredCapabilities[0] = "mutated"
	first[0].Matching.Selector.ExactAttributes["container.image"] = "mutated"
	second, err := registry.ResolveDefinition("redis", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got := second.TemplateSettingRules["labels"].Default.(map[string]any)["team"]; got != "agent" {
		t.Fatalf("default label=%q, want agent", got)
	}
	if got := second.Matching.RequiredCapabilities[0]; got != "redisreceiver" {
		t.Fatalf("required capability=%q, want redisreceiver", got)
	}
	if got := second.Matching.Selector.ExactAttributes["container.image"]; got != "redis" {
		t.Fatalf("image selector=%q, want redis", got)
	}
}
