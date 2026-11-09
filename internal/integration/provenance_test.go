package integration

import (
	"errors"
	"testing"
)

func TestResolverNormalizesLegacySourceAndCopiesResourceBinding(t *testing.T) {
	registry, err := NewRegistry(testDefinition())
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(registry)
	legacy := IntegrationInstance{ID: "cache-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: map[string]any{"endpoint": "localhost:6379"}}
	resolved, err := resolver.Resolve(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Instance.Source != SourceLocal {
		t.Fatalf("legacy source normalized to %q", resolved.Instance.Source)
	}

	binding := &ResourceBinding{Provider: "docker", ProviderInstance: "local-engine", ResourceID: "container-a"}
	discovered := legacy
	discovered.Source = SourceDiscovery
	discovered.Binding = binding
	resolved, err = resolver.Resolve(discovered)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Instance.Binding.ResourceID = "changed"
	if binding.ResourceID != "container-a" || discovered.Binding.ResourceID != "container-a" {
		t.Fatal("resolved instance aliases caller-owned resource binding")
	}
}

func TestValidateInstanceRequiresValidDiscoveryBindingAndKnownSource(t *testing.T) {
	instance := IntegrationInstance{ID: "cache-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: map[string]any{"endpoint": "localhost:6379"}}
	instance.Source = SourceDiscovery
	if err := ValidateInstance(instance); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("discovery source without binding error = %v", err)
	}
	instance.Binding = &ResourceBinding{Provider: "Docker", ProviderInstance: "local", ResourceID: "container-a"}
	if err := ValidateInstance(instance); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("malformed provider error = %v", err)
	}
	instance.Source = "remote"
	instance.Binding = &ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a"}
	if err := ValidateInstance(instance); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("unknown source error = %v", err)
	}
}
