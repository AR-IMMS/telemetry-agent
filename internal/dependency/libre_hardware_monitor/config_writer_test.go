package librehardwaremonitor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteConfigPersistsAgentOwnedConfiguration(t *testing.T) {
	options := DefaultOptions()
	options.InstallDir = t.TempDir()

	want, err := RenderConfig(options)
	if err != nil {
		t.Fatalf("RenderConfig() error = %v", err)
	}

	if err := WriteConfig(options); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	configPath := filepath.Join(
		options.InstallDir,
		"LibreHardwareMonitor.config",
	)

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf(
			"persisted config = %q, want rendered Agent-owned config",
			got,
		)
	}
}
