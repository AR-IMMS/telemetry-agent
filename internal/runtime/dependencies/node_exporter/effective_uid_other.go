//go:build !linux

package nodeexporter

// currentEffectiveUserID rejects direct Node Exporter installation off Linux.
func currentEffectiveUserID() int {
	return -1
}
