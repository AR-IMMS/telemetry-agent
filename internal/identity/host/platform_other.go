//go:build !linux && !windows

package identity

import "fmt"

func collectPlatformDetails() (platformDetails, error) {
	// Refuse unsupported platforms rather than returning an incomplete identity.
	return platformDetails{}, fmt.Errorf("unsupported platform")
}
