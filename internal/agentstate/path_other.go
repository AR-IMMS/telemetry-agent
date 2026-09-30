//go:build !windows && !linux

package agentstate

// DefaultPath returns no state location on unsupported platforms.
func DefaultPath() string {
	return ""
}
