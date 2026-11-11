package collectorruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	redisintegration "github.com/ar-imms/telemetry-agent/internal/integration/redis"
	"gopkg.in/yaml.v3"
)

func testCatalog(t *testing.T) Catalog {
	t.Helper()
	c, err := NewCatalog("0.160.0", "test-manifest", []Capability{{ID: "testreceiver", NativeName: "otlp", Kind: "receiver", OS: []string{"linux"}, Signals: []string{"metrics"}, Pipeline: "metrics"}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testInput(t *testing.T) config.RenderInput {
	t.Helper()
	path := filepath.Join(t.TempDir(), "baseline.yaml")
	body := []byte("receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: 127.0.0.1:4317\n  hostmetrics:\n    collection_interval: 30s\n    scrapers:\n      cpu: {}\n      memory: {}\nprocessors:\n  batch: {}\nexporters:\n  otlp/gateway:\n    endpoint: gateway:4317\nservice:\n  pipelines:\n    metrics:\n      receivers: [otlp, hostmetrics]\n      processors: [batch]\n      exporters: [otlp/gateway]\n    logs:\n      receivers: [otlp]\n      processors: [batch]\n      exporters: [otlp/gateway]\n    traces:\n      receivers: [otlp]\n      processors: [batch]\n      exporters: [otlp/gateway]\n")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return config.RenderInput{Platform: identity.PlatformInfo{OS: "linux", Architecture: "amd64"}, Layers: []config.Layer{{Name: "test", Path: path}}}
}

func testResolved(t *testing.T, id string) integration.ResolvedInstance {
	t.Helper()
	intent, err := integration.NewComponentIntent("testreceiver", map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "127.0.0.1:9000"}}})
	if err != nil {
		t.Fatal(err)
	}
	return integration.ResolvedInstance{Instance: integration.IntegrationInstance{ID: id, DefinitionID: "test", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true}, Contribution: integration.NewContribution([]integration.ComponentIntent{intent})}
}

func TestCompilerIsDeterministicAndPreservesBaselinePipelines(t *testing.T) {
	c := NewCompiler(testCatalog(t))
	input := testInput(t)
	a, err := c.Compile(input, []integration.ResolvedInstance{testResolved(t, "instance-a")})
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Compile(input, []integration.ResolvedInstance{testResolved(t, "instance-a")})
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest() != b.Digest() || !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("equivalent inputs generated different candidate bytes/digests")
	}
	ordered, err := c.Compile(input, []integration.ResolvedInstance{testResolved(t, "instance-z"), testResolved(t, "instance-a")})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := c.Compile(input, []integration.ResolvedInstance{testResolved(t, "instance-a"), testResolved(t, "instance-z")})
	if err != nil {
		t.Fatal(err)
	}
	if ordered.Digest() != reversed.Digest() || !bytes.Equal(ordered.Bytes(), reversed.Bytes()) {
		t.Fatal("intent input order changed candidate bytes/digest")
	}
	for _, required := range [][]byte{[]byte("otlp/gateway"), []byte("hostmetrics"), []byte("traces:"), []byte("logs:"), []byte("gateway:4317"), []byte("otlp/" + "i-")} {
		if !bytes.Contains(a.Bytes(), required) {
			t.Fatalf("candidate lost baseline marker %q", required)
		}
	}
	baselineBytes, err := config.Render(input)
	if err != nil {
		t.Fatal(err)
	}
	var baseline, compiled map[string]any
	if err := yaml.Unmarshal(baselineBytes, &baseline); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(a.Bytes(), &compiled); err != nil {
		t.Fatal(err)
	}
	baseExporters := baseline["exporters"].(map[string]any)
	compiledExporters := compiled["exporters"].(map[string]any)
	if !reflect.DeepEqual(baseExporters["otlp/gateway"], compiledExporters["otlp/gateway"]) {
		t.Fatal("compiler changed the existing Gateway exporter configuration")
	}
	basePipelines := baseline["service"].(map[string]any)["pipelines"].(map[string]any)
	compiledPipelines := compiled["service"].(map[string]any)["pipelines"].(map[string]any)
	for _, signal := range []string{"metrics", "logs", "traces"} {
		before := basePipelines[signal].(map[string]any)
		after := compiledPipelines[signal].(map[string]any)
		if !reflect.DeepEqual(before["processors"], after["processors"]) || !reflect.DeepEqual(before["exporters"], after["exporters"]) {
			t.Fatalf("compiler changed %s processors/exporters", signal)
		}
		for _, receiver := range before["receivers"].([]any) {
			if !containsString(after["receivers"].([]any), receiver.(string)) {
				t.Errorf("compiler removed existing %s receiver %q", signal, receiver)
			}
		}
	}
}

func containsString(values []any, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestCompilerRejectsUnsupportedCapabilityAndAmbiguousKinds(t *testing.T) {
	c := NewCompiler(testCatalog(t))
	unknown, _ := integration.NewComponentIntent("unknownreceiver", map[string]any{})
	resolved := integration.ResolvedInstance{Instance: integration.IntegrationInstance{ID: "cache", DefinitionID: "test", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true}, Contribution: integration.NewContribution([]integration.ComponentIntent{unknown})}
	if _, err := c.Compile(testInput(t), []integration.ResolvedInstance{resolved}); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("unsupported error=%v", err)
	}
	processor, err := NewCatalog("0.160.0", "test", []Capability{{ID: "proc", NativeName: "batch", Kind: "processor", OS: []string{"linux"}, Signals: []string{"metrics"}, Pipeline: "metrics"}})
	if err != nil {
		t.Fatal(err)
	}
	i, err := integration.NewComponentIntent("proc", map[string]any{"timeout": "1s"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewCompiler(processor).Compile(testInput(t), []integration.ResolvedInstance{{Instance: integration.IntegrationInstance{ID: "x", DefinitionID: "test", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true}, Contribution: integration.NewContribution([]integration.ComponentIntent{i})}})
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("ambiguous processor mapping error=%v", err)
	}
}

func TestCompilerWiresExporterWithoutReplacingBaseline(t *testing.T) {
	exporter, err := NewCatalog("0.160.0", "test", []Capability{{ID: "testexporter", NativeName: "otlp", Kind: "exporter", OS: []string{"linux"}, Signals: []string{"metrics"}, Pipeline: "metrics"}})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := integration.NewComponentIntent("testexporter", map[string]any{"endpoint": "127.0.0.1:4317"})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := NewCompiler(exporter).Compile(testInput(t), []integration.ResolvedInstance{{Instance: integration.IntegrationInstance{ID: "cache", DefinitionID: "test", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true}, Contribution: integration.NewContribution([]integration.ComponentIntent{intent})}})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range [][]byte{[]byte("otlp/gateway"), []byte("otlp/i-"), []byte("gateway:4317")} {
		if !bytes.Contains(candidate.Bytes(), required) {
			t.Errorf("candidate lost exporter or baseline marker %q", required)
		}
	}
}

func TestNewCatalogRejectsAmbiguousPipelineMapping(t *testing.T) {
	_, err := NewCatalog("0.160.0", "test", []Capability{{ID: "ambiguous", NativeName: "otlp", Kind: "receiver", OS: []string{"linux"}, Signals: []string{"metrics", "logs"}, Pipeline: "metrics"}})
	if !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("ambiguous mapping error=%v", err)
	}
	_, err = NewCatalog("0.160.0", "test", []Capability{{ID: "wrong-signal", NativeName: "otlp", Kind: "receiver", OS: []string{"linux"}, Signals: []string{"metrics"}, Pipeline: "logs"}})
	if !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("cross-signal mapping error=%v", err)
	}
}

func TestCompilerRejectsDuplicateIntentsAndSkipsDisabledInstances(t *testing.T) {
	catalog := testCatalog(t)
	intent, err := integration.NewComponentIntent("testreceiver", map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "127.0.0.1:9000"}}})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := testResolved(t, "instance-a")
	duplicate.Contribution = integration.NewContribution([]integration.ComponentIntent{intent, intent})
	if _, err := NewCompiler(catalog).Compile(testInput(t), []integration.ResolvedInstance{duplicate}); !errors.Is(err, ErrComponentIDConflict) {
		t.Fatalf("duplicate intent error=%v", err)
	}
	disabled := testResolved(t, "disabled")
	disabled.Instance.Enabled = false
	candidate, err := NewCompiler(catalog).Compile(testInput(t), []integration.ResolvedInstance{disabled})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(candidate.Bytes(), []byte("/i-"+stableInstanceSuffix("disabled"))) {
		t.Fatal("disabled instance emitted Collector configuration")
	}
}

func TestCandidateBytesAreDefensivelyCopied(t *testing.T) {
	candidate, err := NewCompiler(testCatalog(t)).Compile(testInput(t), []integration.ResolvedInstance{testResolved(t, "instance-a")})
	if err != nil {
		t.Fatal(err)
	}
	first := candidate.Bytes()
	first[0] ^= 0xff
	if bytes.Equal(first, candidate.Bytes()) {
		t.Fatal("mutating returned bytes changed immutable candidate content")
	}
}

func TestRedisInstancesCompileToIndependentStableReceiverIDs(t *testing.T) {
	definition := redisintegration.Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolver := integration.NewResolver(registry)
	var resolved []integration.ResolvedInstance
	for _, id := range []string{"redis-a", "redis-b"} {
		instance, err := resolver.Resolve(integration.IntegrationInstance{
			ID: id, DefinitionID: definition.ID, DefinitionVersion: definition.Version,
			Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
		})
		if err != nil {
			t.Fatal(err)
		}
		resolved = append(resolved, instance)
	}
	input := testInput(t)
	input.Platform = identity.PlatformInfo{OS: "windows", Architecture: "amd64"}
	catalog, err := StaticCatalog(input.Platform)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := NewCompiler(catalog).Compile(input, resolved)
	if err != nil {
		t.Fatal(err)
	}
	content := string(candidate.Bytes())
	for _, id := range []string{"redis-a", "redis-b"} {
		want := "redis/i-" + stableInstanceSuffix(id)
		if !strings.Contains(content, want) {
			t.Errorf("candidate missing independent Redis receiver %q", want)
		}
	}
}

func TestCompilerFailsClosedWhenCatalogDoesNotAuthorizeRedis(t *testing.T) {
	definition := redisintegration.Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := integration.NewResolver(registry).Resolve(integration.IntegrationInstance{
		ID: "redis-a", DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCompiler(testCatalog(t)).Compile(testInput(t), []integration.ResolvedInstance{resolved}); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("Redis compile error = %v, want unsupported capability", err)
	}
}

func TestNativeValidationWhenPinnedBinaryProvided(t *testing.T) {
	binary := os.Getenv("OTELCOL_CONTRIB_BINARY")
	if binary == "" {
		t.Skip("pinned Collector binary is not available in this test environment")
	}
	candidate, err := NewCompiler(testCatalog(t)).Compile(testInput(t), []integration.ResolvedInstance{testResolved(t, "instance-a")})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCandidate(context.Background(), bootstrap.OSCommandRunner{}, binary, candidate, nil); err != nil {
		t.Fatalf("pinned Collector rejected candidate: %v", err)
	}
}

func TestRedisCandidateNativeValidationWhenPinnedBinaryProvided(t *testing.T) {
	binary := os.Getenv("OTELCOL_CONTRIB_BINARY")
	if binary == "" {
		t.Skip("pinned Collector binary is not available in this test environment")
	}
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := StaticCatalog(platform)
	if err != nil {
		t.Fatal(err)
	}
	definition := redisintegration.Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := integration.NewResolver(registry).Resolve(integration.IntegrationInstance{
		ID: "redis-native-validation", DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
	})
	if err != nil {
		t.Fatal(err)
	}
	configRoot, err := filepath.Abs(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	input := config.RenderInput{
		Platform: platform,
		Layers: []config.Layer{
			{Name: "base", Path: filepath.Join(configRoot, "base", "otel.yaml")},
			{Name: "profile", Path: filepath.Join(configRoot, "profiles", "laptop.yaml")},
			{Name: "os", Path: filepath.Join(configRoot, "os", platform.OS, "otel.yaml")},
		},
		InlineLayers: []config.InlineLayer{config.MetricsReceiverLayer([]string{"otlp", "hostmetrics"})},
		Values:       map[string]string{"OTEL_GATEWAY_ENDPOINT": "127.0.0.1:4317"},
	}
	candidate, err := NewCompiler(catalog).Compile(input, []integration.ResolvedInstance{resolved})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCandidate(context.Background(), bootstrap.OSCommandRunner{}, binary, candidate, []string{"OTEL_GATEWAY_ENDPOINT=127.0.0.1:4317"}); err != nil {
		t.Fatalf("pinned Collector rejected Redis candidate: %v", err)
	}
}

type validationRunner struct {
	path   string
	result bootstrap.CommandResult
}

func (r *validationRunner) Run(_ context.Context, command bootstrap.Command) bootstrap.CommandResult {
	r.path = command.Args[2]
	if r.result.ExitCode != 0 || r.result.Err != nil {
		return r.result
	}
	return bootstrap.CommandResult{ExitCode: 0}
}

func TestValidateCandidateUsesStagedNonActiveFile(t *testing.T) {
	active := filepath.Join(t.TempDir(), "active.yaml")
	want := []byte("old active config")
	if err := os.WriteFile(active, want, 0600); err != nil {
		t.Fatal(err)
	}
	runner := &validationRunner{}
	candidate, err := NewCompiler(testCatalog(t)).Compile(testInput(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCandidate(context.Background(), runner, "otelcol-contrib", candidate, nil); err != nil {
		t.Fatal(err)
	}
	if runner.path == "" || runner.path == active {
		t.Fatalf("validator path=%q must be staged and non-active", runner.path)
	}
	got, err := os.ReadFile(active)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("active config changed: %q %v", got, err)
	}
}

func TestNativeValidationErrorDoesNotExposeCollectorOutput(t *testing.T) {
	runner := &validationRunner{result: bootstrap.CommandResult{ExitCode: 1, Stderr: "invalid setting contains super-secret-value"}}
	candidate, err := NewCompiler(testCatalog(t)).Compile(testInput(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateCandidate(context.Background(), runner, "otelcol-contrib", candidate, nil)
	if !errors.Is(err, ErrNativeValidation) {
		t.Fatalf("validation error=%v", err)
	}
	if bytes.Contains([]byte(err.Error()), []byte("super-secret-value")) {
		t.Fatalf("validation error exposed Collector output: %v", err)
	}
}
