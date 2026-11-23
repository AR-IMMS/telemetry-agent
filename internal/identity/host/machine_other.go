//go:build !windows && !linux

package identity

import "fmt"

func readRawMachineID() (string, error) {
	return "", fmt.Errorf(
		"machine identity is unsupported on %q",
		"this platform",
	)
}
