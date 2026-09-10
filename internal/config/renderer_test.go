package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeLayer(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "layer.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func decodeYAML(t *testing.T, rendered []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := yaml.Unmarshal(rendered, &document); err != nil {
		t.Fatal(err)
	}
	return document
}
func nested(t *testing.T, document map[string]any, keys ...string) any {
	t.Helper()
	var current any = document
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%v is not a map", key)
		}
		current = m[key]
	}
	return current
}
func list(t *testing.T, document map[string]any, keys ...string) []string {
	t.Helper()
	raw, ok := nested(t, document, keys...).([]any)
	if !ok {
		t.Fatalf("not a list")
	}
	result := make([]string, len(raw))
	for i, value := range raw {
		result[i] = value.(string)
	}
	return result
}

func TestRenderMergesLayers(t *testing.T) {
	rendered, err := Render(RenderInput{Layers: []Layer{{"base", writeLayer(t, "receivers: {otlp: {}}\nprocessors: {base: {}}\nexporters: {gateway: {endpoint: base}}\nservice:\n  pipelines:\n    metrics:\n      receivers: [otlp]\n      exporters: [gateway]\n")}, {"profile", writeLayer(t, "exporters:\n  gateway:\n    endpoint: profile\nprocessors:\n  extra: {}\nservice:\n  pipelines:\n    metrics:\n      receivers: [otlp, hostmetrics]\n      exporters: [gateway]\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	document := decodeYAML(t, rendered)
	if nested(t, document, "exporters", "gateway", "endpoint") != "profile" {
		t.Fatal("scalar overlay failed")
	}
	if nested(t, document, "processors", "base") == nil || nested(t, document, "processors", "extra") == nil {
		t.Fatal("nested merge failed")
	}
	if !slices.Equal(list(t, document, "service", "pipelines", "metrics", "receivers"), []string{"otlp", "hostmetrics"}) {
		t.Fatal("list replacement failed")
	}
}

func TestRenderReturnsContextualErrors(t *testing.T) {
	_, err := Render(RenderInput{Layers: []Layer{{Name: "base", Path: "missing.yaml"}}})
	if err == nil || !strings.Contains(err.Error(), "base") {
		t.Fatalf("error = %v", err)
	}
}
func TestValidateDocumentRejectsMissingPipelines(t *testing.T) {
	err := ValidateDocument(map[string]any{"receivers": map[string]any{"x": map[string]any{}}, "processors": map[string]any{"x": map[string]any{}}, "exporters": map[string]any{"x": map[string]any{}}, "service": map[string]any{}})
	if err == nil || !strings.Contains(err.Error(), "service.pipelines") {
		t.Fatalf("error = %v", err)
	}
}
