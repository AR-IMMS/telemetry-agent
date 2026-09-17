package nodeexporter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemdUnitMatchesDetectsManagedUnitDrift(t *testing.T) {
	options := Options{
		InstallDir:    "/opt/ar-imms/node-exporter",
		ServicePath:   filepath.Join(t.TempDir(), "ar-imms-node-exporter.service"),
		ListenAddress: "127.0.0.1:9100",
	}

	rendered, err := RenderSystemdUnit(options)
	if err != nil {
		t.Fatalf("RenderSystemdUnit() error = %v", err)
	}

	if err := os.WriteFile(options.ServicePath, rendered, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	matches, err := systemdUnitMatches(options)
	if err != nil {
		t.Fatalf("systemdUnitMatches() error = %v", err)
	}
	if !matches {
		t.Fatal("systemdUnitMatches() = false, want true for rendered unit")
	}

	if err := os.WriteFile(
		options.ServicePath,
		[]byte("[Service]\nType=simple\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile() drifted unit error = %v", err)
	}

	matches, err = systemdUnitMatches(options)
	if err != nil {
		t.Fatalf("systemdUnitMatches() drift error = %v", err)
	}
	if matches {
		t.Fatal("systemdUnitMatches() = true, want false for drifted unit")
	}
}
