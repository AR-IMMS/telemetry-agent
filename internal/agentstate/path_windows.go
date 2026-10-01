//go:build windows

package agentstate

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath returns the machine-wide Agent state path on Windows.
func DefaultPath() string {
	programData := strings.TrimSpace(os.Getenv("ProgramData"))
	if programData == "" {
		programData = `C:\ProgramData`
	}

	return filepath.Join(
		programData,
		"AR-IMMS",
		"Telemetry Agent",
		"state.json",
	)
}
