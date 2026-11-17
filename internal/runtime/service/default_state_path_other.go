//go:build !windows && !linux

package agentinstallation

// DefaultStatePath returns no machine-wide path on unsupported platforms.
func DefaultStatePath() string {
	return ""
}
