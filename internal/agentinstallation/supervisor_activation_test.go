package agentinstallation

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestSupervisorActivationRequiresExplicitEndpointAndPreservesExistingMode(t *testing.T) {
	endpoint := "wss://opamp.example.test/v1/opamp"
	if gotURL, enabled := supervisorActivationForInstall("", agentstate.State{}); gotURL != "" || enabled {
		t.Fatalf("fresh install without endpoint = (%q, %t), want inactive", gotURL, enabled)
	}
	if gotURL, enabled := supervisorActivationForInstall(endpoint, agentstate.State{}); gotURL != endpoint || !enabled {
		t.Fatalf("explicit fresh install = (%q, %t), want endpoint enabled", gotURL, enabled)
	}
	previous := agentstate.State{Installation: &agentstate.AgentInstallation{}, Supervisor: agentstate.SupervisorContext{OpAMPServerURL: endpoint, Enabled: true}}
	if gotURL, enabled := supervisorActivationForInstall("", previous); gotURL != endpoint || !enabled {
		t.Fatalf("upgrade without endpoint = (%q, %t), want existing owner preserved", gotURL, enabled)
	}
	if gotURL, enabled := supervisorActivationForInstall(endpoint, agentstate.State{Installation: &agentstate.AgentInstallation{}}); gotURL != endpoint || !enabled {
		t.Fatalf("explicit ownership transfer = (%q, %t), want enabled", gotURL, enabled)
	}
}
