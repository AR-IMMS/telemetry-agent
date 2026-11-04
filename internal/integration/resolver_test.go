package integration

import (
	"errors"
	"reflect"
	"testing"
)

func TestRegistryUsesExactVersionAndRejectsDuplicates(t *testing.T) {
	definition := testDefinition()
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ResolveDefinition("redis", "2.0.0"); !errors.Is(err, ErrUnknownDefinition) {
		t.Fatalf("version mismatch error = %v", err)
	}
	if _, err := NewRegistry(definition, definition); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestResolverDeterministicDisabledAndTypedFailures(t *testing.T) {
	definition := testDefinition()
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(registry)
	instance := IntegrationInstance{ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}}
	a, err := resolver.Resolve(instance)
	if err != nil {
		t.Fatal(err)
	}
	b, err := resolver.Resolve(instance)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Contribution.Intents()[0].Configuration(), b.Contribution.Intents()[0].Configuration()) {
		t.Fatal("same input produced different contributions")
	}
	instance.Enabled = false
	disabled, err := resolver.Resolve(instance)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled.Contribution.Intents()) != 0 {
		t.Fatal("disabled instance contributed intents")
	}
	instance.Enabled = true
	instance.DefinitionVersion = "9.0.0"
	if _, err := resolver.Resolve(instance); !errors.Is(err, ErrUnknownDefinition) {
		t.Fatalf("unknown definition error = %v", err)
	}
	instance.DefinitionVersion = "1.0.0"
	instance.Configuration["endpoint"] = 12
	if _, err := resolver.Resolve(instance); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("validation error = %v", err)
	}
}

func TestResolverClassifiesContributorErrors(t *testing.T) {
	definition := testDefinition()
	definition.Contribute = func(IntegrationInstance) (Contribution, error) {
		return Contribution{}, errors.New("invalid contribution")
	}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	instance := IntegrationInstance{ID: "cache", DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true, Configuration: map[string]any{"endpoint": "localhost"}}
	if _, err := NewResolver(registry).Resolve(instance); !errors.Is(err, ErrInvalidContribution) {
		t.Fatalf("contribution error = %v", err)
	}
}
