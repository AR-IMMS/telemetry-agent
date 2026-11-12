package agentlifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestCleanBaselineSnapshotIncludesManagedReceiversAndFingerprintsLayers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.yaml")
	if err := os.WriteFile(path, []byte("receivers: {otlp: {}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := dependency.NewCatalog([]dependency.Integration{{
		Definition:        dependency.Definition{Name: "test-exporter", DisplayName: "Test Exporter", SupportedOS: []string{"linux"}},
		CollectorReceiver: "prometheus/test_exporter",
		Install:           func(context.Context) (dependency.InstallResult, error) { return dependency.InstallResult{}, nil },
		Inspect:           func(context.Context) (dependency.Inspection, error) { return dependency.Inspection{}, nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	state := agentstate.State{
		DesiredGeneration: 7,
		Dependencies: map[string]agentstate.DependencyState{
			"test-exporter": {Enabled: true},
		},
	}
	platform := identity.PlatformInfo{OS: "linux", Architecture: "amd64"}
	layers := []config.Layer{{Name: "base", Path: path}}

	first, err := BuildCleanBaselineSnapshot(platform, catalog, layers, state, nil)
	if err != nil {
		t.Fatalf("BuildCleanBaselineSnapshot() error = %v", err)
	}
	if first.DesiredGeneration != 7 {
		t.Fatalf("DesiredGeneration = %d, want 7", first.DesiredGeneration)
	}
	if len(first.Input.InlineLayers) != 1 {
		t.Fatalf("InlineLayers = %d, want managed receivers only", len(first.Input.InlineLayers))
	}
	if _, exists := first.Input.InlineLayers[0].Document["extensions"]; exists {
		t.Fatal("clean baseline unexpectedly contains generation readiness overlay")
	}
	receivers := first.Input.InlineLayers[0].Document["service"].(map[string]any)["pipelines"].(map[string]any)["metrics"].(map[string]any)["receivers"].([]any)
	if got, want := receivers, []any{"otlp", "hostmetrics", "prometheus/test_exporter"}; !equalAnySlice(got, want) {
		t.Fatalf("baseline receivers = %#v, want %#v", got, want)
	}

	if err := os.WriteFile(path, []byte("receivers: {otlp: {protocols: {grpc: {endpoint: '127.0.0.1:4317'}}}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := BuildCleanBaselineSnapshot(platform, catalog, layers, state, nil)
	if err != nil {
		t.Fatalf("BuildCleanBaselineSnapshot() after layer change error = %v", err)
	}
	if second.Fingerprint == first.Fingerprint {
		t.Fatal("Fingerprint unchanged after baseline layer content changed")
	}
	state.DesiredGeneration++
	third, err := BuildCleanBaselineSnapshot(platform, catalog, layers, state, nil)
	if err != nil {
		t.Fatalf("BuildCleanBaselineSnapshot() after generation change error = %v", err)
	}
	if third.Fingerprint != second.Fingerprint {
		t.Fatal("baseline content fingerprint changed when only the separately tracked M2 generation advanced")
	}
	withValue, err := BuildCleanBaselineSnapshot(platform, catalog, layers, state, map[string]string{"OTEL_GATEWAY_ENDPOINT": "gateway-a:4317"})
	if err != nil {
		t.Fatal(err)
	}
	otherValue, err := BuildCleanBaselineSnapshot(platform, catalog, layers, state, map[string]string{"OTEL_GATEWAY_ENDPOINT": "gateway-b:4317"})
	if err != nil {
		t.Fatal(err)
	}
	if withValue.Fingerprint == otherValue.Fingerprint {
		t.Fatal("baseline fingerprint unchanged after a relevant render value changed")
	}
}

func TestCleanBaselineSnapshotIncludesNativeMonitoringInputsInConfigurationAndFingerprint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.yaml")
	if err := os.WriteFile(path, []byte("extensions: {health_check: {endpoint: 127.0.0.1:13133}}\nservice: {extensions: [health_check], pipelines: {metrics: {receivers: [otlp]}}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	platform := identity.PlatformInfo{OS: "windows", Architecture: "amd64"}
	layers := []config.Layer{{Name: "base", Path: path}}
	state := agentstate.State{}
	nativeLayer := config.InlineLayer{Name: "native_docker", Document: map[string]any{
		"extensions": map[string]any{"docker_observer": map[string]any{"endpoint": "npipe:////./pipe/docker_engine"}},
		"service":    map[string]any{"extensions": []any{"health_check", "docker_observer"}},
	}}
	first, err := BuildCleanBaselineSnapshotWithAdditionalInputs(platform, dependency.Catalog{}, layers, state, nil, []config.InlineLayer{nativeLayer}, []string{"receiver_creator"})
	if err != nil {
		t.Fatal(err)
	}
	receivers := first.Input.InlineLayers[0].Document["service"].(map[string]any)["pipelines"].(map[string]any)["metrics"].(map[string]any)["receivers"].([]any)
	if got, want := receivers, []any{"otlp", "hostmetrics", "receiver_creator"}; !equalAnySlice(got, want) {
		t.Fatalf("metrics receivers=%#v, want %#v", got, want)
	}
	second, err := BuildCleanBaselineSnapshot(platform, dependency.Catalog{}, layers, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("baseline fingerprint ignored the native monitoring policy inputs")
	}
}

func equalAnySlice(left, right []any) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
