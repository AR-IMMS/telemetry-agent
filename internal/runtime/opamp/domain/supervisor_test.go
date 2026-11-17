package opampdomain

import (
	"encoding/json"
	"testing"
)

func TestSupervisorContextKeepsPersistedJSONShape(t *testing.T) {
	context := SupervisorContext{
		BinaryPath:        `C:\Agent\opampsupervisor.exe`,
		ConfigurationPath: `C:\Agent\opamp.yaml`,
		StateDirectory:    `C:\ProgramData\AR-IMMS\opamp`,
		OpAMPServerURL:    "wss://control.example/opamp",
		ServerCAFile:      `C:\Agent\certs\ca.pem`,
		Enabled:           true,
	}

	encoded, err := json.Marshal(context)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	const want = `{"binaryPath":"C:\\Agent\\opampsupervisor.exe","configurationPath":"C:\\Agent\\opamp.yaml","stateDirectory":"C:\\ProgramData\\AR-IMMS\\opamp","opampServerUrl":"wss://control.example/opamp","serverCaFile":"C:\\Agent\\certs\\ca.pem","enabled":true}`
	if string(encoded) != want {
		t.Fatalf("SupervisorContext JSON = %s, want %s", encoded, want)
	}
}
