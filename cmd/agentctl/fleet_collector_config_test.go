package main

import (
	"path/filepath"
	"testing"

	config "github.com/ar-imms/telemetry-agent/internal/runtime/collector/configuration"
)

func TestFleetCollectorLayerRequiresProtectedOTLPTokenPath(t *testing.T) {
	base := []config.Layer{{Name: "base", Path: "base.yaml"}}
	if got := fleetCollectorLayers(base, "configs", ""); len(got) != 1 {
		t.Fatalf("unenrolled configuration layers = %v", got)
	}
	got := fleetCollectorLayers(base, "configs", "C:/protected/otlp-collector.token")
	if len(got) != 2 || got[1].Name != "fleet" || got[1].Path != filepath.Join("configs", "fleet", "otel.yaml") {
		t.Fatalf("Fleet configuration layers = %v", got)
	}
}
