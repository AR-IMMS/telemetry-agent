package integration

import (
	"errors"
	"testing"
)

func TestRegistryEnumeratesOneMatchableVersionAndPreservesExactVersions(t *testing.T) {
	matchable := testDefinition()
	matchable.Matching = &MatchingDefinition{Selector: &MatchSelector{ResourceKinds: []string{"container"}}}
	manual := testDefinition()
	manual.Version = "2.0.0"
	registry, err := NewRegistry(manual, matchable)
	if err != nil {
		t.Fatal(err)
	}
	definitions := registry.MatchableDefinitions()
	if len(definitions) != 1 || definitions[0].ID != "redis" || definitions[0].Version != "1.0.0" {
		t.Fatalf("matchable definitions=%+v", definitions)
	}
	if _, err := registry.ResolveDefinition("redis", "2.0.0"); err != nil {
		t.Fatalf("exact manual version lookup failed: %v", err)
	}
	conflict := testDefinition()
	conflict.Version = "3.0.0"
	conflict.Matching = &MatchingDefinition{Selector: &MatchSelector{ResourceKinds: []string{"container"}}}
	if _, err := NewRegistry(matchable, conflict); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("duplicate matchable-version error=%v", err)
	}
}
