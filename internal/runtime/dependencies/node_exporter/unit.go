package nodeexporter

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// systemdUnitExists reports whether the Agent-owned systemd unit file exists.
func systemdUnitExists(unitPath string) (bool, error) {
	unitPath = strings.TrimSpace(unitPath)

	if unitPath == "" {
		return false, fmt.Errorf("Node Exporter systemd unit path is required")
	}

	info, err := os.Stat(unitPath)

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"inspect Node Exporter systemd unit %q: %w",
			unitPath,
			err,
		)
	}
	if info.IsDir() {
		return false, fmt.Errorf(
			"Node Exporter systemd unit path %q is a directory",
			unitPath,
		)
	}

	return true, nil
}
