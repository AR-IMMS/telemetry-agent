package app

import (
	"errors"
	"testing"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	definitionsapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestMonitoringDesiredStateResolverDefaultsSourceAndPreservesBinding(t *testing.T) {
	resolver := testMonitoringDesiredStateResolver(t)
	binding := &desiredstatedomain.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "redis-1"}
	instance := desiredstatedomain.Instance{
		ID: "redis-local", DefinitionID: "redis", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Binding: binding, Enabled: true,
	}

	resolved, contribution, err := resolver.ResolveWithContribution(instance)
	if err != nil {
		t.Fatalf("ResolveWithContribution() error = %v", err)
	}
	if resolved.Source != desiredstatedomain.SourceLocal {
		t.Fatalf("resolved source = %q, want local", resolved.Source)
	}
	if resolved.Configuration["endpoint"] != "127.0.0.1:6379" {
		t.Fatalf("resolved settings = %#v, want validated endpoint", resolved.Configuration)
	}
	if resolved.Binding == binding || resolved.Binding == nil || *resolved.Binding != *binding {
		t.Fatalf("resolved binding = %#v, want copied value %#v", resolved.Binding, binding)
	}
	if len(contribution.Intents()) != 1 || contribution.Intents()[0].Capability() != "redisreceiver" {
		t.Fatalf("contribution = %#v, want redisreceiver intent", contribution.Intents())
	}
}

func TestMonitoringDesiredStateResolverPreservesErrorOwnership(t *testing.T) {
	resolver := testMonitoringDesiredStateResolver(t)
	instance := desiredstatedomain.Instance{
		ID: "redis-local", DefinitionID: "missing-profile", DefinitionVersion: "1.0.0",
		Configuration: map[string]any{}, Enabled: true,
	}
	if _, _, err := resolver.ResolveWithContribution(instance); !errors.Is(err, definitionsdomain.ErrUnknownDefinition) {
		t.Fatalf("unknown definition error = %v, want Monitoring's unknown-definition error", err)
	}

	instance.DefinitionID = "redis"
	instance.Configuration = map[string]any{"endpoint": ""}
	if _, _, err := resolver.ResolveWithContribution(instance); !errors.Is(err, desiredstatedomain.ErrInvalidInstance) {
		t.Fatalf("invalid settings error = %v, want Configuration's invalid-instance classification", err)
	}
}

func testMonitoringDesiredStateResolver(t *testing.T) MonitoringDesiredStateResolver {
	t.Helper()
	registry, err := definitionsapp.NewRegistry(definitionsdomain.Definition{
		ID: "redis", Version: "1.0.0",
		Validate: func(settings map[string]any) error {
			if settings["endpoint"] != "127.0.0.1:6379" {
				return errors.New("invalid endpoint")
			}
			return nil
		},
		Contribute: func(map[string]any) (definitionsdomain.Contribution, error) {
			intent, err := definitionsdomain.NewComponentIntent("redisreceiver", map[string]any{"endpoint": "127.0.0.1:6379"})
			if err != nil {
				return definitionsdomain.Contribution{}, err
			}
			return definitionsdomain.NewContribution([]definitionsdomain.ComponentIntent{intent}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewMonitoringDesiredStateResolver(definitionsapp.NewResolver(registry))
}
