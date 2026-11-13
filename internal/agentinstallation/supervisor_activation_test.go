package agentinstallation

import (
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestSupervisorActivationDefaultsNewInstallsToSupervisor(t *testing.T) {
	endpoint := "wss://opamp.example.test/v1/opamp"
	if _, enabled, err := supervisorActivationForInstall("", agentstate.State{}, false); err == nil || enabled {
		t.Fatalf("fresh install without endpoint = enabled %t, error %v; want required endpoint", enabled, err)
	}
	if gotURL, enabled, err := supervisorActivationForInstall(endpoint, agentstate.State{}, false); gotURL != endpoint || !enabled || err != nil {
		t.Fatalf("fresh default install = (%q, %t), error %v; want Supervisor enabled", gotURL, enabled, err)
	}
	managed := agentstate.State{Installation: &agentstate.AgentInstallation{}, Supervisor: agentstate.SupervisorContext{OpAMPServerURL: endpoint, Enabled: true}}
	if gotURL, enabled, err := supervisorActivationForInstall("", managed, false); gotURL != endpoint || !enabled || err != nil {
		t.Fatalf("managed upgrade without endpoint = (%q, %t), error %v; want existing owner preserved", gotURL, enabled, err)
	}
	legacy := agentstate.State{Installation: &agentstate.AgentInstallation{}}
	if gotURL, enabled, err := supervisorActivationForInstall("", legacy, false); gotURL != "" || enabled || err != nil {
		t.Fatalf("legacy upgrade without endpoint = (%q, %t), error %v; want legacy owner preserved", gotURL, enabled, err)
	}
	if gotURL, enabled, err := supervisorActivationForInstall(endpoint, legacy, false); gotURL != endpoint || !enabled || err != nil {
		t.Fatalf("explicit migration = (%q, %t), error %v; want Supervisor enabled", gotURL, enabled, err)
	}
	if gotURL, enabled, err := supervisorActivationForInstall("", managed, true); gotURL != "" || enabled || err != nil {
		t.Fatalf("explicit rollback = (%q, %t), error %v; want legacy owner", gotURL, enabled, err)
	}
	if gotURL, enabled, err := supervisorActivationForInstall("", agentstate.State{}, true); gotURL != "" || enabled || err != nil {
		t.Fatalf("explicit fresh legacy selection = (%q, %t), error %v; want legacy owner", gotURL, enabled, err)
	}
}
