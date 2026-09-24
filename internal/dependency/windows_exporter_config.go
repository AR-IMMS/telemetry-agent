package dependency

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/config"
)

// RenderWindowsExporterConfig creates the complete Agent-owned exporter config.
func RenderWindowsExporterConfig(
	options WindowsExporterOptions,
) ([]byte, error) {
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validate Windows Exporter configuration options: %w",
			err,
		)
	}

	rendered := fmt.Sprintf(`collectors:
  enabled: "[defaults]"
telemetry:
  path: /metrics
web:
  listen-address: %s
`, strings.TrimSpace(options.ListenAddress))

	return []byte(rendered), nil
}

// WriteWindowsExporterConfig atomically persists config before MSI consumes it.
func WriteWindowsExporterConfig(
	options WindowsExporterOptions,
) error {
	rendered, err := RenderWindowsExporterConfig(options)
	if err != nil {
		return err
	}

	if err := config.WriteAtomic(options.ConfigPath, rendered, 0600); err != nil {
		return fmt.Errorf("write Windows Exporter configuration: %w", err)
	}

	return nil
}

// WindowsExporterConfigMatches reports whether the Agent-owned configuration
// file exactly matches the configuration rendered from the supplied options.
func WindowsExporterConfigMatches(
	options WindowsExporterOptions,
) (bool, error) {
	expected, err := RenderWindowsExporterConfig(options)
	if err != nil {
		return false, err
	}

	actual, err := os.ReadFile(options.ConfigPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf(
			"read Windows Exporter configuration: %w",
			err,
		)
	}

	return bytes.Equal(actual, expected), nil
}
