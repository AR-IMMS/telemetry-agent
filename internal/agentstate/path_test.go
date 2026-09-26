package agentstate

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultPathUsesPlatformOwnedStateLocation(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		programData := t.TempDir()
		t.Setenv("ProgramData", programData)

		want := filepath.Join(
			programData,
			"AR-IMMS",
			"agent",
			"state.json",
		)

		if got := DefaultPath(); got != want {
			t.Fatalf("DefaultPath() = %q, want %q", got, want)
		}

	case "linux":
		const want = "/var/lib/ar-imms/agent/state.json"

		if got := DefaultPath(); got != want {
			t.Fatalf("DefaultPath() = %q, want %q", got, want)
		}

	default:
		t.Skipf("unsupported test platform %q", runtime.GOOS)
	}
}
