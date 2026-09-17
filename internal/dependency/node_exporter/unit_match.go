package nodeexporter

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

// systemdUnitMatches reports whether the Agent-owned unit on disk matches the
// exact unit rendered by the current Agent release.
func systemdUnitMatches(options Options) (bool, error) {
	if err := options.Validate(); err != nil {
		return false, fmt.Errorf(
			"validate Node Exporter unit options: %w",
			err,
		)
	}

	expected, err := RenderSystemdUnit(options)
	if err != nil {
		return false, fmt.Errorf(
			"render expected Node Exporter systemd unit: %w",
			err,
		)
	}

	actual, err := os.ReadFile(options.ServicePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"read Node Exporter systemd unit: %w",
			err,
		)
	}

	return bytes.Equal(actual, expected), nil
}
