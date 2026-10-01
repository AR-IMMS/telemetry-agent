//go:build linux

package agentstate

// DefaultPath returns the machine-wide Agent state path on Linux.
func DefaultPath() string {
	return "/var/lib/ar-imms/telemetry-agent/state.json"
}
