//go:build windows

package identity

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	windowsMachineGUIDPath = `SOFTWARE\Microsoft\Cryptography`
	windowsMachineGUIDName = "MachineGuid"
)

func readRawMachineID() (string, error) {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		windowsMachineGUIDPath,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return "", fmt.Errorf("open Windows MachineGuid key: %w", err)
	}
	defer key.Close()

	value, _, err := key.GetStringValue(windowsMachineGUIDName)
	if err != nil {
		return "", fmt.Errorf("read Windows MachineGuid: %w", err)
	}

	return value, nil
}
