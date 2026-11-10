package main

import (
	"testing"
)

func TestProductionIntegrationRegistryKeepsIdentificationIndependentFromCapabilities(t *testing.T) {
	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ResolveDefinition("redis", "1.0.0"); err != nil {
		t.Fatalf("Redis identification definition not registered: %v", err)
	}
	definitions := registry.MatchableDefinitions()
	if len(definitions) != 1 || definitions[0].ID != "redis" {
		t.Fatalf("matchable definitions=%+v", definitions)
	}
}
