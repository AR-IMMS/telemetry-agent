//go:build linux

package nodeexporter

import "os"

// currentEffectiveUserID returns the current Linux effective user ID.
func currentEffectiveUserID() int {
	return os.Geteuid()
}
