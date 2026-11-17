package agentinstallation

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultStatePathUsesRuntimeOwnedPlatformLocation(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		programData := t.TempDir()
		t.Setenv("ProgramData", programData)
		want := filepath.Join(programData, "AR-IMMS", "Telemetry Agent", "state.db")
		if got := DefaultStatePath(); got != want {
			t.Fatalf("DefaultStatePath() = %q, want %q", got, want)
		}
	case "linux":
		const want = "/var/lib/ar-imms/telemetry-agent/state.db"
		if got := DefaultStatePath(); got != want {
			t.Fatalf("DefaultStatePath() = %q, want %q", got, want)
		}
	default:
		t.Skipf("unsupported test platform %q", runtime.GOOS)
	}
}
