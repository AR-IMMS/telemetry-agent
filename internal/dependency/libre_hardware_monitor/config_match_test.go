package librehardwaremonitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigMatchesRecognizesRequiredManagedSettings(t *testing.T) {
	options := DefaultOptions()
	options.InstallDir = t.TempDir()

	if err := WriteConfig(options); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	matches, err := ConfigMatches(options)
	if err != nil {
		t.Fatalf("ConfigMatches() error = %v", err)
	}
	if !matches {
		t.Fatal("ConfigMatches() = false, want true")
	}
}

func TestConfigMatchesDetectsManagedPortDrift(t *testing.T) {
	options := DefaultOptions()
	options.InstallDir = t.TempDir()

	if err := WriteConfig(options); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	configPath := filepath.Join(
		options.InstallDir,
		"LibreHardwareMonitor.config",
	)

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	drifted := strings.Replace(
		string(content),
		`value="9190"`,
		`value="9191"`,
		1,
	)

	if err := os.WriteFile(configPath, []byte(drifted), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	matches, err := ConfigMatches(options)
	if err != nil {
		t.Fatalf("ConfigMatches() error = %v", err)
	}
	if matches {
		t.Fatal("ConfigMatches() = true, want false for listener port drift")
	}
}
