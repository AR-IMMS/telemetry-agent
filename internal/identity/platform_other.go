//go:build !linux && !windows

package identity

import "fmt"

func collectPlatformDetails() (platformDetails, error) {
	return platformDetails{}, fmt.Errorf("unsupported platform")
}
