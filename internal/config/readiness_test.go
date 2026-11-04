package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReadinessLayerEmbedsCandidateToken(t *testing.T) {
	const token = "candidate-token"
	document := map[string]any{
		"extensions": map[string]any{"health_check": map[string]any{"endpoint": "127.0.0.1:13133"}},
	}
	merged := mergeDocument(document, ReadinessLayer(token).Document)
	content, err := yaml.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadinessTokenFromConfig(content)
	if err != nil || got != token {
		t.Fatalf("ReadinessTokenFromConfig() = %q, %v; want %q", got, err, token)
	}
	if merged["extensions"].(map[string]any)["health_check"].(map[string]any)["endpoint"] != "127.0.0.1:13133" {
		t.Fatal("readiness overlay changed the shared health endpoint")
	}
}

func TestReadinessTokenFromLegacyConfigIsEmpty(t *testing.T) {
	got, err := ReadinessTokenFromConfig([]byte("extensions:\n  health_check:\n    endpoint: 127.0.0.1:13133\n"))
	if err != nil || got != "" {
		t.Fatalf("ReadinessTokenFromConfig() = %q, %v; want empty legacy token", got, err)
	}
}

func TestWithReadinessTokenPreservesExistingHealthCheckSettings(t *testing.T) {
	legacy := []byte("extensions:\n  health_check:\n    endpoint: 127.0.0.1:13133\nservice:\n  extensions: [health_check]\n")
	updated, err := WithReadinessToken(legacy, "candidate-token")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadinessTokenFromConfig(updated)
	if err != nil || got != "candidate-token" {
		t.Fatalf("ReadinessTokenFromConfig() = %q, %v", got, err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	if nestedValue(document, "extensions", "health_check", "endpoint") != "127.0.0.1:13133" {
		t.Fatal("health-check endpoint was not preserved")
	}
	if nestedValue(document, "service", "extensions").([]any)[0] != "health_check" {
		t.Fatal("service health-check extension was not preserved")
	}
}
