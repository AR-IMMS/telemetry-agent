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

func TestRenderLinuxConfigKeepsBaseExporterAndLaptopProcessors(
	t *testing.T,
) {
	root := t.TempDir()

	base := writePlatformContractLayer(t, root, "base/otel.yaml", `
receivers:
  otlp: {}
processors:
  memory_limiter: {}
  batch: {}
exporters:
  otlp/gateway:
    endpoint: ${env:OTEL_GATEWAY_ENDPOINT}
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp/gateway]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp/gateway]
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp/gateway]
`)

	laptop := writePlatformContractLayer(t, root, "profiles/laptop.yaml", `
processors:
  resource/laptop:
    attributes: []
service:
  pipelines:
    metrics:
      processors: [memory_limiter, resource/laptop, batch]
    logs:
      processors: [memory_limiter, resource/laptop, batch]
    traces:
      processors: [memory_limiter, resource/laptop, batch]
`)

	linux := writePlatformContractLayer(t, root, "os/linux/otel.yaml", `
receivers:
  hostmetrics: {}
service:
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
`)

	rendered, err := Render(RenderInput{
		Layers: []Layer{base, laptop, linux},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	document := decodeRenderedDocument(t, rendered)

	assertPipelineList(
		t,
		document,
		"metrics",
		"receivers",
		[]string{"otlp", "hostmetrics"},
	)
	assertPipelineList(
		t,
		document,
		"metrics",
		"processors",
		[]string{"memory_limiter", "resource/laptop", "batch"},
	)

	assertPipelineList(
		t,
		document,
		"logs",
		"receivers",
		[]string{"otlp"},
	)
	assertPipelineList(
		t,
		document,
		"traces",
		"receivers",
		[]string{"otlp"},
	)

	assertGatewayExporterForAllPipelines(t, document)
}

func TestRenderWindowsConfigKeepsBaseExporterAndLaptopProcessors(
	t *testing.T,
) {
	root := t.TempDir()

	base := writePlatformContractLayer(t, root, "base/otel.yaml", `
receivers:
  otlp: {}
processors:
  memory_limiter: {}
  batch: {}
exporters:
  otlp/gateway:
    endpoint: ${env:OTEL_GATEWAY_ENDPOINT}
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp/gateway]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp/gateway]
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp/gateway]
`)

	laptop := writePlatformContractLayer(t, root, "profiles/laptop.yaml", `
processors:
  resource/laptop:
    attributes: []
service:
  pipelines:
    metrics:
      processors: [memory_limiter, resource/laptop, batch]
    logs:
      processors: [memory_limiter, resource/laptop, batch]
    traces:
      processors: [memory_limiter, resource/laptop, batch]
`)

	windows := writePlatformContractLayer(t, root, "os/windows/otel.yaml", `
receivers:
  prometheus/windows_exporter:
    config:
      scrape_configs: []
service:
  pipelines:
    metrics:
      receivers: [otlp, prometheus/windows_exporter]
`)

	rendered, err := Render(RenderInput{
		Layers: []Layer{base, laptop, windows},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	document := decodeRenderedDocument(t, rendered)

	assertPipelineList(
		t,
		document,
		"metrics",
		"receivers",
		[]string{"otlp", "prometheus/windows_exporter"},
	)
	assertPipelineList(
		t,
		document,
		"metrics",
		"processors",
		[]string{"memory_limiter", "resource/laptop", "batch"},
	)

	assertPipelineList(
		t,
		document,
		"logs",
		"receivers",
		[]string{"otlp"},
	)
	assertPipelineList(
		t,
		document,
		"traces",
		"receivers",
		[]string{"otlp"},
	)

	assertGatewayExporterForAllPipelines(t, document)
}

func writePlatformContractLayer(
	t *testing.T,
	root string,
	relativePath string,
	content string,
) Layer {
	t.Helper()

	path := filepath.Join(root, relativePath)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return Layer{
		Name: relativePath,
		Path: path,
	}
}

func decodeRenderedDocument(
	t *testing.T,
	rendered []byte,
) map[string]any {
	t.Helper()

	var document map[string]any

	if err := yaml.Unmarshal(rendered, &document); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	return document
}

func assertGatewayExporterForAllPipelines(
	t *testing.T,
	document map[string]any,
) {
	t.Helper()

	for _, pipelineName := range []string{"metrics", "logs", "traces"} {
		assertPipelineList(
			t,
			document,
			pipelineName,
			"exporters",
			[]string{"otlp/gateway"},
		)
	}
}

func assertPipelineList(
	t *testing.T,
	document map[string]any,
	pipelineName string,
	field string,
	want []string,
) {
	t.Helper()

	service, ok := document["service"].(map[string]any)
	if !ok {
		t.Fatal("service is not a map")
	}

	pipelines, ok := service["pipelines"].(map[string]any)
	if !ok {
		t.Fatal("service.pipelines is not a map")
	}

	pipeline, ok := pipelines[pipelineName].(map[string]any)
	if !ok {
		t.Fatalf("pipeline %q is not a map", pipelineName)
	}

	rawList, ok := pipeline[field].([]any)
	if !ok {
		t.Fatalf(
			"pipeline %q field %q is not a list",
			pipelineName,
			field,
		)
	}

	got := make([]string, 0, len(rawList))

	for _, item := range rawList {
		value, ok := item.(string)
		if !ok {
			t.Fatalf(
				"pipeline %q field %q contains non-string value %T",
				pipelineName,
				field,
				item,
			)
		}

		got = append(got, value)
	}

	if len(got) != len(want) {
		t.Fatalf(
			"pipeline %q field %q = %v, want %v",
			pipelineName,
			field,
			got,
			want,
		)
	}

	for index := range want {
		if got[index] != want[index] {
			t.Fatalf(
				"pipeline %q field %q = %v, want %v",
				pipelineName,
				field,
				got,
				want,
			)
		}
	}
}
