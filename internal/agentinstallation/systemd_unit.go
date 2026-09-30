package agentinstallation

import (
	"fmt"
	"strings"
)

const systemdUnitTemplate = `[Unit]
Description=AR-IMMS Telemetry Agent
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStart=%s run --state-path %s
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`

// RenderSystemdUnit returns the systemd unit content for a Linux Agent
// installation. It performs no filesystem or process operations.
func RenderSystemdUnit(layout Layout) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(layout.Platform), "linux") {
		return "", fmt.Errorf(
			"systemd unit requires Linux layout, got %q",
			layout.Platform,
		)
	}

	if strings.TrimSpace(layout.AgentBinaryPath) == "" {
		return "", fmt.Errorf("Agent binary path is required")
	}
	if strings.TrimSpace(layout.StatePath) == "" {
		return "", fmt.Errorf("Agent state path is required")
	}

	return fmt.Sprintf(
		systemdUnitTemplate,
		layout.AgentBinaryPath,
		layout.StatePath,
	), nil
}
