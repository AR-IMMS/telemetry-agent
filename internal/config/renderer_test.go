package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
	"gopkg.in/yaml.v3"
)

func writeLayer(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "layer.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
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

func TestRenderSubstitutesPlatformIdentityValues(t *testing.T) {
	layer := Layer{
		Name: "base",
		Path: writeLayer(t, `
receivers:
  otlp: {}
processors:
  resource/agent_identity:
    attributes:
      - key: host.name
        value: "${value:host.name}"
        action: upsert
      - key: host.id
        value: "${value:host.id}"
        action: upsert
      - key: service.name
        value: ar-imms-node-agent
        action: insert
exporters:
  debug: {}
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [resource/agent_identity]
      exporters: [debug]
`),
	}

	rendered, err := Render(RenderInput{
		Platform: identity.PlatformInfo{
			Hostname: "MSI",
			HostID:   "windows-abf00cac9374d455fd349d0c",
		},
		Layers: []Layer{layer},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	document := decodeYAML(t, rendered)

	attributes, ok := nested(
		t,
		document,
		"processors",
		"resource/agent_identity",
		"attributes",
	).([]any)
	if !ok {
		t.Fatal("resource/agent_identity.attributes is not a list")
	}

	got := make(map[string]string, len(attributes))

	for _, rawAttribute := range attributes {
		attribute, ok := rawAttribute.(map[string]any)
		if !ok {
			t.Fatalf("attribute type = %T, want map", rawAttribute)
		}

		key, _ := attribute["key"].(string)
		value, _ := attribute["value"].(string)
		got[key] = value
	}

	if got["host.name"] != "MSI" {
		t.Fatalf("host.name = %q, want %q", got["host.name"], "MSI")
	}

	if got["host.id"] != "windows-abf00cac9374d455fd349d0c" {
		t.Fatalf("host.id = %q, want hashed host ID", got["host.id"])
	}
}

func TestRenderProductionLinuxConfigIncludesHostMetrics(t *testing.T) {
	document := assertProductionHostMetricsConfig(
		t,
		"linux",
		[]string{
			"otlp",
			"hostmetrics",
			"prometheus/node_exporter",
		},
	)

	scrapers, ok := nested(
		t,
		document,
		"receivers",
		"hostmetrics",
		"scrapers",
	).(map[string]any)
	if !ok {
		t.Fatal("hostmetrics.scrapers is not a map")
	}

	if _, enabled := scrapers["process"]; enabled {
		t.Fatal(
			"Linux hostmetrics must not enable the process scraper without privileged runtime",
		)
	}
}

func TestRenderProductionWindowsConfigIncludesHostMetrics(t *testing.T) {
	assertProductionHostMetricsConfig(
		t,
		"windows",
		[]string{"otlp", "hostmetrics", "prometheus/windows_exporter"},
	)
}

func assertProductionHostMetricsConfig(
	t *testing.T,
	osName string,
	wantReceivers []string,
) map[string]any {
	t.Helper()

	configRoot := filepath.Join("..", "..", "configs")

	rendered, err := Render(RenderInput{
		Layers: []Layer{
			{
				Name: "base",
				Path: filepath.Join(configRoot, "base", "otel.yaml"),
			},
			{
				Name: "profile",
				Path: filepath.Join(configRoot, "profiles", "laptop.yaml"),
			},
			{
				Name: "os",
				Path: filepath.Join(configRoot, "os", osName, "otel.yaml"),
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	document := decodeYAML(t, rendered)

	hostmetrics, ok := nested(
		t,
		document,
		"receivers",
		"hostmetrics",
	).(map[string]any)
	if !ok {
		t.Fatal("receivers.hostmetrics is not configured")
	}

	if got := hostmetrics["collection_interval"]; got != "15s" {
		t.Fatalf(
			"hostmetrics collection_interval = %v, want 15s",
			got,
		)
	}

	scrapers, ok := hostmetrics["scrapers"].(map[string]any)
	if !ok {
		t.Fatal("receivers.hostmetrics.scrapers is not a map")
	}

	for _, scraper := range []string{
		"cpu",
		"memory",
		"disk",
		"filesystem",
		"network",
		"paging",
	} {
		if _, exists := scrapers[scraper]; !exists {
			t.Fatalf(
				"receivers.hostmetrics.scrapers.%s is not configured",
				scraper,
			)
		}
	}

	assertPipelineList(
		t,
		document,
		"metrics",
		"receivers",
		wantReceivers,
	)

	return document
}

func TestRenderProductionGatewayExporterUsesPlaintextForHomelabPoC(
	t *testing.T,
) {
	configRoot := filepath.Join("..", "..", "configs")

	rendered, err := Render(RenderInput{
		Layers: []Layer{
			{
				Name: "base",
				Path: filepath.Join(configRoot, "base", "otel.yaml"),
			},
			{
				Name: "profile",
				Path: filepath.Join(configRoot, "profiles", "laptop.yaml"),
			},
			{
				Name: "os",
				Path: filepath.Join(configRoot, "os", "linux", "otel.yaml"),
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	document := decodeYAML(t, rendered)

	tls, ok := nested(
		t,
		document,
		"exporters",
		"otlp/gateway",
		"tls",
	).(map[string]any)
	if !ok {
		t.Fatal("exporters.otlp/gateway.tls is not configured")
	}

	if got := tls["insecure"]; got != true {
		t.Fatalf(
			"exporters.otlp/gateway.tls.insecure = %v, want true",
			got,
		)
	}
}

func TestRenderRepositoryWindowsConfigScrapesWindowsExporter(
	t *testing.T,
) {
	configRoot := filepath.Join("..", "..", "configs")

	rendered, err := Render(RenderInput{
		Platform: identity.PlatformInfo{
			OS:           "windows",
			Architecture: "amd64",
			Hostname:     "test-windows-node",
			HostID:       "windows-test-node",
		},
		Layers: []Layer{
			{
				Name: "base",
				Path: filepath.Join(configRoot, "base", "otel.yaml"),
			},
			{
				Name: "profile",
				Path: filepath.Join(
					configRoot,
					"profiles",
					"laptop.yaml",
				),
			},
			{
				Name: "os",
				Path: filepath.Join(
					configRoot,
					"os",
					"windows",
					"otel.yaml",
				),
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	document := decodeRenderedDocument(t, rendered)

	receivers, ok := document["receivers"].(map[string]any)
	if !ok {
		t.Fatal("receivers is not a map")
	}

	if _, ok := receivers["prometheus/windows_exporter"]; !ok {
		t.Fatal(
			"prometheus/windows_exporter receiver is missing from Windows config",
		)
	}

	assertPipelineList(
		t,
		document,
		"metrics",
		"receivers",
		[]string{
			"otlp",
			"hostmetrics",
			"prometheus/windows_exporter",
		},
	)
}

func TestRenderRepositoryLinuxConfigScrapesNodeExporter(
	t *testing.T,
) {
	configRoot := filepath.Join("..", "..", "configs")

	rendered, err := Render(RenderInput{
		Platform: identity.PlatformInfo{
			OS:           "linux",
			Architecture: "amd64",
			Hostname:     "test-linux-node",
			HostID:       "linux-test-node",
		},
		Layers: []Layer{
			{
				Name: "base",
				Path: filepath.Join(configRoot, "base", "otel.yaml"),
			},
			{
				Name: "profile",
				Path: filepath.Join(
					configRoot,
					"profiles",
					"laptop.yaml",
				),
			},
			{
				Name: "os",
				Path: filepath.Join(
					configRoot,
					"os",
					"linux",
					"otel.yaml",
				),
			},
		},
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	document := decodeRenderedDocument(t, rendered)

	receivers, ok := document["receivers"].(map[string]any)
	if !ok {
		t.Fatal("receivers is not a map")
	}

	if _, ok := receivers["prometheus/node_exporter"]; !ok {
		t.Fatal(
			"prometheus/node_exporter receiver is missing from Linux config",
		)
	}

	assertPipelineList(
		t,
		document,
		"metrics",
		"receivers",
		[]string{
			"otlp",
			"hostmetrics",
			"prometheus/node_exporter",
		},
	)
}
