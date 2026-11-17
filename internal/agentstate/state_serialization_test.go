package agentstate

import (
	"encoding/json"
	"testing"

	activationdomain "github.com/ar-imms/telemetry-agent/internal/runtime/collector/activation/domain"
	servicedomain "github.com/ar-imms/telemetry-agent/internal/runtime/service/domain"
)

func TestAgentInstallationOwnershipJSONRemainsCompatible(t *testing.T) {
	installation := servicedomain.AgentInstallation{
		Platform: "windows", ServiceName: "agent-service", BinaryPath: `C:\Agent\agent.exe`,
		Ownership: servicedomain.OwnershipRecord{Resources: []servicedomain.OwnedResource{{Kind: "file", Identifier: `C:\Agent\agent.exe`}}},
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

func TestPendingActivationStateJSONRemainsCompatible(t *testing.T) {
	state := State{
		DesiredGeneration:   5,
		ActivatedGeneration: 4,
		AppliedGeneration:   3,
		PendingActivation: &activationdomain.PendingActivation{
			Generation:   5,
			RecoveryPath: `C:\Agent\last-good.yaml`,
			Candidate: &activationdomain.CandidateMapping{
				DesiredSetRevision:    4,
				BaselineFingerprint:   "baseline",
				CandidateDigest:       "candidate",
				EffectiveConfigDigest: "effective",
				CollectorVersion:      "0.160.0",
				ManifestVersion:       "v1",
				Platform:              "windows",
			},
		},
	}

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	const want = `{"collector":{"configRoot":"","binaryPath":"","configPath":"","gatewayEndpoint":"","healthEndpoint":""},"supervisor":{},"desiredGeneration":5,"activatedGeneration":4,"appliedGeneration":3,"pendingActivation":{"generation":5,"recoveryPath":"C:\\Agent\\last-good.yaml","candidate":{"desiredSetRevision":4,"baselineFingerprint":"baseline","candidateDigest":"candidate","effectiveConfigDigest":"effective","collectorVersion":"0.160.0","manifestVersion":"v1","platform":"windows"}},"dependencies":null}`
	if string(encoded) != want {
		t.Fatalf("state JSON = %s, want %s", encoded, want)
	}
}
