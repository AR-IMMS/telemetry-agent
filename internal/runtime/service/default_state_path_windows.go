//go:build windows

package agentinstallation

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultStatePath returns the machine-wide Agent state database path on Windows.
func DefaultStatePath() string {
	programData := strings.TrimSpace(os.Getenv("ProgramData"))
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "AR-IMMS", "Telemetry Agent", "state.db")
}
