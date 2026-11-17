package agentstate

import (
	"encoding/json"
	"testing"
)

func TestAgentInstallationOwnershipJSONRemainsCompatible(t *testing.T) {
	installation := AgentInstallation{
		Platform: "windows", ServiceName: "agent-service", BinaryPath: `C:\Agent\agent.exe`,
		Ownership: OwnershipRecord{Resources: []OwnedResource{{Kind: "file", Identifier: `C:\Agent\agent.exe`}}},
	}
	encoded, err := json.Marshal(installation)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want := `{"platform":"windows","serviceName":"agent-service","binaryPath":"C:\\Agent\\agent.exe","ownership":{"resources":[{"kind":"file","identifier":"C:\\Agent\\agent.exe"}]}}`
	if string(encoded) != want {
		t.Fatalf("installation JSON = %s, want %s", encoded, want)
	}
}
