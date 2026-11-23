//go:build linux

package agentinstallation

// DefaultStatePath returns the machine-wide Agent state database path on Linux.
func DefaultStatePath() string {
	return "/var/lib/ar-imms/telemetry-agent/state.db"
}
