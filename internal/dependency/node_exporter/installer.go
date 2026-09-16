package nodeexporter

import (
	"fmt"
	"path"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/config"
)

// Options controls the Agent-owned Node Exporter installation.
type Options struct {
	// InstallDir stores the verified Node Exporter binary.
	InstallDir string

	// ServicePath is the Agent-owned systemd unit file.
	ServicePath string

	// ListenAddress is the loopback Prometheus metrics endpoint.
	ListenAddress string
}

// Validate rejects incomplete options before installation changes the host.
func (o Options) Validate() error {
	if strings.TrimSpace(o.InstallDir) == "" {
		return fmt.Errorf("Node Exporter installation directory is required")
	}
	if strings.TrimSpace(o.ServicePath) == "" {
		return fmt.Errorf("Node Exporter service path is required")
	}
	if strings.TrimSpace(o.ListenAddress) == "" {
		return fmt.Errorf("Node Exporter listen address is required")
	}

	return nil
}

// DefaultOptions returns the Agent-owned Node Exporter runtime locations.
func DefaultOptions() Options {
	return Options{
		InstallDir:    "/opt/ar-imms/node-exporter",
		ServicePath:   "/etc/systemd/system/ar-imms-node-exporter.service",
		ListenAddress: "127.0.0.1:9100",
	}
}

// RenderSystemdUnit creates the Agent-owned Node Exporter systemd unit.
func RenderSystemdUnit(options Options) ([]byte, error) {
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf(
			"validate Node Exporter systemd options: %w",
			err,
		)
	}

	binaryPath := path.Join(
		strings.TrimSpace(options.InstallDir),
		"node_exporter",
	)

	unit := fmt.Sprintf(`[Unit]
Description=AR-IMMS Node Exporter
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --web.listen-address=%s
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`,
		binaryPath,
		strings.TrimSpace(options.ListenAddress),
	)

	return []byte(unit), nil
}

// WriteSystemdUnit renders and atomically persists the Agent-owned systemd unit.
func WriteSystemdUnit(options Options) error {
	rendered, err := RenderSystemdUnit(options)
	if err != nil {
		return err
	}

	if err := config.WriteAtomic(
		options.ServicePath,
		rendered,
		0o644,
	); err != nil {
		return fmt.Errorf(
			"write Node Exporter systemd unit: %w",
			err,
		)
	}

	return nil
}
