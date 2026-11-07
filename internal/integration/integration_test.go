package integration

import (
	"errors"
	"testing"
)

func testDefinition() IntegrationDefinition {
	return IntegrationDefinition{ID: "redis", Version: "1.0.0", Validate: func(config map[string]any) error {
		if _, ok := config["endpoint"].(string); !ok {
			return errors.New("endpoint required")
		}
		return nil
	}, Contribute: func(instance IntegrationInstance) (Contribution, error) {
		intent, err := NewComponentIntent("redisreceiver", map[string]any{"endpoint": instance.Configuration["endpoint"]})
		if err != nil {
			return Contribution{}, err
		}
		return NewContribution([]ComponentIntent{intent}), nil
	}}
}

func TestValidateDefinitionAndInstanceRejectMalformedAndSecretSettings(t *testing.T) {
	definition := testDefinition()
	if err := ValidateDefinition(definition); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		run    func() error
		target error
	}{
		{"definition ID", func() error { d := definition; d.ID = "Redis"; return ValidateDefinition(d) }, ErrInvalidDefinition},
		{"definition version", func() error { d := definition; d.Version = "1.0"; return ValidateDefinition(d) }, ErrInvalidDefinition},
		{"instance ID", func() error {
			return ValidateInstance(IntegrationInstance{ID: "bad id", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: map[string]any{}})
		}, ErrInvalidInstance},
		{"plaintext credential", func() error {
			return ValidateInstance(IntegrationInstance{ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: map[string]any{"endpoint": "localhost", "api_key": "plaintext"}})
		}, ErrInvalidInstance},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, test.target) {
				t.Fatalf("error = %v, want %v", err, test.target)
			}
		})
	}
}

func TestComponentIntentAndContributionCopyMutableData(t *testing.T) {
	config := map[string]any{"nested": map[string]any{"value": "before"}}
	intent, err := NewComponentIntent("redisreceiver", config)
	if err != nil {
		t.Fatal(err)
	}
	config["nested"].(map[string]any)["value"] = "after"
	first := intent.Configuration()
	first[0] = 'x'
	contribution := NewContribution([]ComponentIntent{intent})
	copyOfIntents := contribution.Intents()
	copyOfIntents[0].config[0] = 'x'
	if got := string(contribution.Intents()[0].Configuration()); got != `{"nested":{"value":"before"}}` {
		t.Fatalf("configuration = %s", got)
	}
	if _, err := NewComponentIntent("redisreceiver", map[string]any{"password": "secret"}); !errors.Is(err, ErrInvalidContribution) {
		t.Fatalf("secret error = %v", err)
	}
}

func TestComponentIntentAcceptsDottedRuntimeCapabilityID(t *testing.T) {
	intent, err := NewComponentIntent("otel.receiver.redis.metrics", map[string]any{"endpoint": "127.0.0.1:6379"})
	if err != nil {
		t.Fatalf("NewComponentIntent() error = %v", err)
	}
	if intent.Capability() != "otel.receiver.redis.metrics" {
		t.Fatalf("capability = %q", intent.Capability())
	}
}
