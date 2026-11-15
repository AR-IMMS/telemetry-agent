//go:build linux

package identity

import (
	"fmt"
	"os"
)

const linuxMachineIDPath = "/etc/machine-id"

func readRawMachineID() (string, error) {
	value, err := os.ReadFile(linuxMachineIDPath)
	if err != nil {
		return "", fmt.Errorf("read Linux machine ID: %w", err)
	}

	return string(value), nil
}
