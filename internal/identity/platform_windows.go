//go:build windows

package identity

import "runtime"

func collectPlatformDetails() (platformDetails, error) {
	return platformDetails{Kernel: runtime.GOOS}, nil
}
