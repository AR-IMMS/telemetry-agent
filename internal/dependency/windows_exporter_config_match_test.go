package dependency

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsExporterConfigMatchesRecognizesAgentOwnedConfig(
	t *testing.T,
) {
	options := DefaultWindowsExporterOptions()
	options.ConfigPath = filepath.Join(t.TempDir(), "config.yaml")

	rendered, err := RenderWindowsExporterConfig(options)
	if err != nil {
		t.Fatalf("RenderWindowsExporterConfig() error = %v", err)
	}

	if err := os.WriteFile(options.ConfigPath, rendered, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	matches, err := WindowsExporterConfigMatches(options)
	if err != nil {
		t.Fatalf("WindowsExporterConfigMatches() error = %v", err)
	}
	if !matches {
		t.Fatal("WindowsExporterConfigMatches() = false, want true")
	}
}

func TestWindowsExporterConfigMatchesTreatsMissingConfigAsDrift(
	t *testing.T,
) {
	options := DefaultWindowsExporterOptions()
	options.ConfigPath = filepath.Join(t.TempDir(), "missing.yaml")

	matches, err := WindowsExporterConfigMatches(options)
	if err != nil {
		t.Fatalf("WindowsExporterConfigMatches() error = %v", err)
	}
	if matches {
		t.Fatal("WindowsExporterConfigMatches() = true, want false")
	}
}
