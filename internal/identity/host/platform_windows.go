//go:build windows

package identity

import "runtime"

func collectPlatformDetails() (platformDetails, error) {
	// Windows-specific enrichment is intentionally minimal until a stable source is selected.
	return platformDetails{Kernel: runtime.GOOS}, nil
}
