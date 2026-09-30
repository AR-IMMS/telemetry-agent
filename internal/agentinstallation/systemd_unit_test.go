package agentinstallation

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestRenderSystemdUnitRunsAgentWithOwnedState(t *testing.T) {
	layout, err := DefaultLayout(identity.PlatformInfo{
		OS: "linux",
	})
	if err != nil {
		t.Fatalf("DefaultLayout() error = %v", err)
	}

	got, err := RenderSystemdUnit(layout)
	if err != nil {
		t.Fatalf("RenderSystemdUnit() error = %v", err)
	}

	want := `[Unit]
Description=AR-IMMS Telemetry Agent
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStart=/opt/ar-imms/telemetry-agent/agentctl run --state-path /var/lib/ar-imms/telemetry-agent/state.json
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`

	if got != want {
		t.Fatalf("RenderSystemdUnit() = %q, want %q", got, want)
	}
}
