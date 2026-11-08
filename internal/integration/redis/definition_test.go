package redis

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestDefinitionValidatesEndpointAndRejectsUnsupportedSettings(t *testing.T) {
	definition := Definition()
	valid := []map[string]any{
		{"endpoint": "127.0.0.1:6379"},
		{"endpoint": "cache.internal:6379"},
		{"endpoint": "[::1]:6379"},
	}
	for _, configuration := range valid {
		if err := definition.Validate(configuration); err != nil {
			t.Errorf("Validate(%v) error = %v", configuration, err)
		}
	}

	invalid := []map[string]any{
		{},
		{"endpoint": "redis://127.0.0.1:6379"},
		{"endpoint": "user:secret@127.0.0.1:6379"},
		{"endpoint": "127.0.0.1:0"},
		{"endpoint": "127.0.0.1:65536"},
		{"endpoint": "bad host:6379"},
		{"endpoint": "bad_host:6379"},
		{"endpoint": "127.0.0.1:6379", "password": "secret"},
	}
	for _, configuration := range invalid {
		if err := definition.Validate(configuration); err == nil {
			t.Errorf("Validate(%v) accepted invalid settings", configuration)
		}
	}
}

func TestDefinitionContributesDeterministicEndpointOnlyIntent(t *testing.T) {
	definition := Definition()
	instance := integration.IntegrationInstance{
		ID: "cache-a", DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
	}
	first, err := definition.Contribute(instance)
	if err != nil {
		t.Fatal(err)
	}
	second, err := definition.Contribute(instance)
	if err != nil {
		t.Fatal(err)
	}
	firstIntents, secondIntents := first.Intents(), second.Intents()
	if len(firstIntents) != 1 || len(secondIntents) != 1 {
		t.Fatalf("intent counts = %d, %d; want one", len(firstIntents), len(secondIntents))
	}
	if firstIntents[0].Capability() != redisMetricsCapability || secondIntents[0].Capability() != redisMetricsCapability {
		t.Fatalf("capabilities = %q and %q", firstIntents[0].Capability(), secondIntents[0].Capability())
	}
	if string(firstIntents[0].Configuration()) != string(secondIntents[0].Configuration()) {
		t.Fatal("contribution is not deterministic")
	}
	var settings map[string]any
	if err := json.Unmarshal(firstIntents[0].Configuration(), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 || settings["endpoint"] != "127.0.0.1:6379" {
		t.Fatalf("intent settings = %#v", settings)
	}
}

func TestDefinitionRejectsSecretBearingSettingsAtDomainBoundary(t *testing.T) {
	definition := Definition()
	_, err := definition.Contribute(integration.IntegrationInstance{
		ID: "cache-a", DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379", "password": "plaintext"},
	})
	if !errors.Is(err, integration.ErrInvalidInstance) {
		t.Fatalf("secret-bearing instance error = %v", err)
	}
}
